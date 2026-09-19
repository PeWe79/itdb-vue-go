package server

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// handleImportDatabase 接收上传的 .db 文件替换当前数据库
func (a *App) handleImportDatabase(w http.ResponseWriter, r *http.Request) {
	dbPath := strings.TrimSpace(a.cfg.DBPath)
	if dbPath == "" || strings.EqualFold(dbPath, ":memory:") {
		writeError(w, http.StatusBadRequest, "In-memory databases do not support import operations.")
		return
	}

	// 限制上传大小 500MB
	r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "File too large or invalid format")
		return
	}

	uploaded, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "File not found")
		return
	}
	defer uploaded.Close()

	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "itdb-import-*.db")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create temporary file.")
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, uploaded); err != nil {
		tmpFile.Close()
		writeError(w, http.StatusInternalServerError, "Failed to save uploaded file.")
		return
	}
	tmpFile.Close()

	// 验证上传的文件是有效的 ITDB 数据库
	if err := validateSQLiteFile(tmpPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 加锁，防止替换期间有其他请求访问数据库
	a.dbMu.Lock()
	defer a.dbMu.Unlock()

	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to resolve database path")
		return
	}

	// 备份当前数据库到 data/backups/ 目录（VACUUM INTO）
	backupPath, err := backupDatabaseBeforeAlter(a.db, dbPath, "import-database")
	if err != nil {
		log.Printf("Pre-import backup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to backup current database, import cancelled")
		return
	}
	log.Printf("Pre-import backup completed: %s", backupPath)

	// 关闭当前数据库连接
	a.db.Close()

	// 清理 WAL 相关文件
	os.Remove(absPath + "-wal")
	os.Remove(absPath + "-shm")

	// 直接覆盖写入新文件
	if err := copyFile(tmpPath, absPath); err != nil {
		log.Printf("Copy imported database file failed: %v, rolling back from backup", err)
		os.Remove(absPath)
		copyFile(backupPath, absPath)
		a.db, _ = sql.Open("sqlite", dbPath)
		setupSQLite(a.db)
		a.db.SetMaxOpenConns(1)
		a.db.SetMaxIdleConns(1)
		a.db.SetConnMaxLifetime(0)
		a.db.SetConnMaxIdleTime(0)
		writeError(w, http.StatusInternalServerError, "Failed to import database, original database has been restored")
		return
	}

	// 打开新数据库
	newDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("Open imported database failed: %v, rolling back from backup", err)
		os.Remove(absPath)
		copyFile(backupPath, absPath)
		a.db, _ = sql.Open("sqlite", dbPath)
		setupSQLite(a.db)
		a.db.SetMaxOpenConns(1)
		a.db.SetMaxIdleConns(1)
		a.db.SetConnMaxLifetime(0)
		a.db.SetConnMaxIdleTime(0)
		writeError(w, http.StatusInternalServerError, "Failed to import database, original database has been restored")
		return
	}

	newDB.SetMaxOpenConns(1)
	newDB.SetMaxIdleConns(1)
	newDB.SetConnMaxLifetime(0)
	newDB.SetConnMaxIdleTime(0)
	if err := setupSQLite(newDB); err != nil {
		log.Printf("Configure imported database failed: %v, rolling back from backup", err)
		newDB.Close()
		os.Remove(absPath)
		copyFile(backupPath, absPath)
		a.db, _ = sql.Open("sqlite", dbPath)
		setupSQLite(a.db)
		a.db.SetMaxOpenConns(1)
		a.db.SetMaxIdleConns(1)
		a.db.SetConnMaxLifetime(0)
		a.db.SetConnMaxIdleTime(0)
		writeError(w, http.StatusInternalServerError, "Failed to import database, original database has been restored")
		return
	}

	a.db = newDB

	// 迁移旧平台数据库结构（处理 settings 和 statustypes 表差异）
	if err := migrateOldDatabaseSchema(newDB); err != nil {
		log.Printf("Database schema migration failed after import: %v, rolling back from backup", err)
		newDB.Close()
		os.Remove(absPath)
		copyFile(backupPath, absPath)
		a.db, _ = sql.Open("sqlite", dbPath)
		setupSQLite(a.db)
		a.db.SetMaxOpenConns(1)
		a.db.SetMaxIdleConns(1)
		a.db.SetConnMaxLifetime(0)
		a.db.SetConnMaxIdleTime(0)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Database structure migration failed.: %v, the original database has been restored.", err))
		return
	}

	if err := ensureItemTypeSoftwareDefaults(newDB); err != nil {
		log.Printf("item type software defaults failed after import: %v, rolling back", err)
		newDB.Close()
		os.Remove(absPath)
		copyFile(backupPath, absPath)
		a.db, _ = sql.Open("sqlite", dbPath)
		setupSQLite(a.db)
		a.db.SetMaxOpenConns(1)
		a.db.SetMaxIdleConns(1)
		a.db.SetConnMaxLifetime(0)
		a.db.SetConnMaxIdleTime(0)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Initialization of software defaults for the hardware type failed.: %v, the original database has been restored.", err))
		return
	}

	// 检查导入的数据库是否有用户，没有则自动创建 admin
	var userCount int64
	if err := newDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err == nil && userCount == 0 {
		adminPass, err := hashPassword("admin123")
		if err == nil {
			newDB.Exec(`INSERT INTO users (username, userdesc, pass, usertype) VALUES (?, ?, ?, ?)`,
				"admin", "administrator", adminPass, 0)
			log.Printf("Imported database has no users, default admin account created")
		}
	}

	log.Printf("Database import completed, backup file: %s", backupPath)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Database imported successfully"})
}

// validateSQLiteFile 验证文件是有效的 ITDB SQLite 数据库
func validateSQLiteFile(path string) error {
	testDB, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("Failed to open database file")
	}
	defer testDB.Close()

	var result string
	if err := testDB.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("Database integrity check failed")
	}
	if result != "ok" {
		return fmt.Errorf("Database integrity check failed: %s", result)
	}

	var name string
	err = testDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='items' LIMIT 1").Scan(&name)
	if err != nil {
		return fmt.Errorf("Uploaded database is missing the 'items' table, not a valid ITDB database")
	}
	return nil
}

// copyFile 跨分区安全复制文件
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

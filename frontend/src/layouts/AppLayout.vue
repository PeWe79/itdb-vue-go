<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router';
import api from '../api/client';
import { useAuthStore } from '../stores/auth';
import { useNoticeStore } from '../stores/notice';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const notice = useNoticeStore();

type ViewHistoryEntry = {
  id: number;
  url: string;
  description: string;
};

const recentHistoryResourceTitleMap: Record<string, string> = {
  items: 'hardware',
  software: 'software',
  invoices: 'invoices',
  agents: 'agents',
  files: 'files',
  contracts: 'contracts',
  locations: 'locations',
  users: 'users',
  racks: 'racks',
};

const recentHistoryDictionaryTitleMap: Record<string, string> = {
  itemtypes: 'hardware types',
  contracttypes: 'contract types',
  statustypes: 'status types',
  filetypes: 'file types',
  dpttypes: 'department types',
  tags: 'tags',
};

const mainNavItems = [
  { to: '/dashboard', label: 'Front Page' },
  { to: '/resources/items', label: 'Hardware', tooltip: 'Hardware List' },
  { to: '/resources/software', label: 'Software', tooltip: 'Software List' },
  { to: '/resources/invoices', label: 'Invoices', tooltip: 'Invoice List' },
  {
    to: '/resources/agents',
    label: 'Agents',
    tooltip: 'Suppliers/Procurement Parties/Contractors/Manufacturers',
  },
  {
    to: '/resources/files',
    label: 'Files',
    tooltip: 'Documents, Manuals, Purchase Orders, Licenses, ...',
  },
  {
    to: '/resources/contracts',
    label: 'Contracts',
    tooltip: 'Support & Maintenance, Leasing, ...',
  },
  { to: '/resources/locations', label: 'Locations' },
  { to: '/resources/users', label: 'Users' },
  { to: '/resources/racks', label: 'Racks' },
];

const dictionaryNavItems = [
  { to: '/dictionaries/itemtypes', label: 'Hardware Types' },
  { to: '/dictionaries/contracttypes', label: 'Contract Types' },
  { to: '/dictionaries/statustypes', label: 'Status Types' },
  { to: '/dictionaries/filetypes', label: 'File Types' },
  { to: '/dictionaries/dpttypes', label: 'Department Types' },
  { to: '/dictionaries/tags', label: 'Tags' },
];

const toolNavItems = [
  { to: '/labels', label: 'Print Labels' },
  { to: '/reports', label: 'Reports' },
  { to: '/browse', label: 'Browse Data' },
  { to: '/settings', label: 'Settings' },
  { to: '/history', label: 'Operation Log' },
];

const dbFileInput = ref<HTMLInputElement | null>(null);
const importingDatabase = ref(false);

function triggerDatabaseImport() {
  dbFileInput.value?.click();
}

async function handleDatabaseFileSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;

  if (!file.name.endsWith('.db')) {
    notice.error('Please select a .db format database file');
    return;
  }

  importingDatabase.value = true;
  notice.info('Importing database, please wait...');

  try {
    const fd = new FormData();
    fd.append('file', file);
    await api.post('/import/database', fd, { timeout: 0 });
    notice.success('Database imported successfully, redirecting to login page...');
    auth.logout();
    setTimeout(() => {
      window.location.href = '/login';
    }, 1500);
  } catch {
    // api client 拦截器已处理错误提示
  } finally {
    importingDatabase.value = false;
  }
}

const viewRefreshTick = ref(0);
const downloadingDatabaseBackup = ref(false);
const downloadingFullBackup = ref(false);
const recentViewHistory = ref<ViewHistoryEntry[]>([]);
const recentHistoryQuickTip = 'Log records of add and edit operations.';
// Use path-based key so same-path query changes (e.g. create=1 cleanup) do not remount the page.
const routeKey = computed(() => `${route.path}::${viewRefreshTick.value}`);

function onNavClick(target: string, event: MouseEvent) {
  const resolved = router.resolve(target);
  if (resolved.path !== route.path) return;

  // 当前菜单重复点击时，强制重建右侧视图
  event.preventDefault();
  viewRefreshTick.value += 1;
}

function logout() {
  auth.logout();
  router.replace('/login');
}

function formatBackupDate() {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${year}${month}${day}`;
}

function resolveDownloadFileName(disposition: unknown) {
  if (typeof disposition !== 'string') return '';
  const utf8 = disposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1]);
    } catch {
      return utf8[1];
    }
  }
  const plain = disposition.match(/filename="?([^";]+)"?/i);
  return plain?.[1] ?? '';
}

async function triggerDownload(
  url: string,
  fallbackFileName: string,
  pendingRef: typeof downloadingDatabaseBackup,
  failedText: string
) {
  if (pendingRef.value) return;

  pendingRef.value = true;
  try {
    const response = await api.get(url, {
      responseType: 'blob',
      timeout: 0,
    });
    const fileName =
      resolveDownloadFileName(response.headers['content-disposition']) || fallbackFileName;
    const contentType = response.headers['content-type'] || 'application/octet-stream';
    const blob =
      response.data instanceof Blob
        ? response.data
        : new Blob([response.data], { type: contentType });
    const objectURL = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = objectURL;
    link.download = fileName;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(objectURL);
  } catch {
    notice.error(failedText);
  } finally {
    pendingRef.value = false;
  }
}

async function downloadDatabaseBackup() {
  await triggerDownload(
    '/backups/database',
    `itdb-${formatBackupDate()}.db`,
    downloadingDatabaseBackup,
    'Database backup download failed.'
  );
}

async function downloadFullBackup() {
  await triggerDownload(
    '/backups/full',
    `itdb-${formatBackupDate()}.tar.gz`,
    downloadingFullBackup,
    'Full backup download failed.'
  );
}

async function loadRecentViewHistory() {
  try {
    const { data } = await api.get<ViewHistoryEntry[]>('/view-history');
    recentViewHistory.value = Array.isArray(data) ? data : [];
  } catch {
    recentViewHistory.value = [];
  }
}

function handleViewHistoryUpdated() {
  void loadRecentViewHistory();
}

function formatRecentViewHistory(entry: ViewHistoryEntry) {
  try {
    const parsed = new URL(entry.url, window.location.origin);
    const pathname = parsed.pathname.replace(/\/+$/, '');
    const editID = parsed.searchParams.get('edit');
    const subtypeEditID = parsed.searchParams.get('subtypeEdit');

    if (pathname.startsWith('/resources/')) {
      const resourceKey = pathname.split('/').filter(Boolean).pop() ?? '';
      const title = recentHistoryResourceTitleMap[resourceKey];
      if (title && editID) return `${title}: ${editID}`;
    }

    if (pathname.startsWith('/dictionaries/')) {
      if (subtypeEditID) return `Contract Subtype: ${subtypeEditID}`;
      const dictionaryKey = pathname.split('/').filter(Boolean).pop() ?? '';
      const title = recentHistoryDictionaryTitleMap[dictionaryKey];
      if (title && editID) return `${title}: ${editID}`;
    }
  } catch {
    // Fallback to stored description below.
  }
  return entry.description;
}

// 空闲 1 小时自动跳转登录页
const IDLE_TIMEOUT = 60 * 60 * 1000;
let idleTimer: ReturnType<typeof setTimeout> | null = null;

function resetIdleTimer() {
  if (idleTimer) clearTimeout(idleTimer);
  idleTimer = setTimeout(() => {
    auth.logout();
    router.replace('/login');
  }, IDLE_TIMEOUT);
}

const idleEvents = ['mousemove', 'keydown', 'click', 'scroll'] as const;

onMounted(() => {
  void loadRecentViewHistory();
  window.addEventListener('itdb:view-history-updated', handleViewHistoryUpdated);
  resetIdleTimer();
  idleEvents.forEach(e => window.addEventListener(e, resetIdleTimer));
});

onBeforeUnmount(() => {
  window.removeEventListener('itdb:view-history-updated', handleViewHistoryUpdated);
  if (idleTimer) clearTimeout(idleTimer);
  idleEvents.forEach(e => window.removeEventListener(e, resetIdleTimer));
});
</script>

<template>
  <div class="app-shell">
    <aside class="app-sidebar">
      <div class="brand-block">
        <img class="brand-block-logo" src="/images/logo.svg" alt="ITDB" />
        <h1>Asset Management System</h1>
      </div>

      <nav class="nav-list">
        <RouterLink
          v-for="item in mainNavItems"
          :key="item.to"
          :to="item.to"
          class="nav-link"
          :class="{ 'quick-tip': !!item.tooltip }"
          :data-quick-tip="item.tooltip || null"
          @click="onNavClick(item.to, $event)"
        >
          {{ item.label }}
        </RouterLink>

        <hr class="nav-sep" />

        <RouterLink
          v-for="item in dictionaryNavItems"
          :key="item.to"
          :to="item.to"
          class="nav-link"
          @click="onNavClick(item.to, $event)"
        >
          {{ item.label }}
        </RouterLink>

        <hr class="nav-sep" />

        <RouterLink
          v-for="item in toolNavItems"
          :key="item.to"
          :to="item.to"
          class="nav-link"
          @click="onNavClick(item.to, $event)"
        >
          {{ item.label }}
        </RouterLink>
        <button
          type="button"
          class="nav-link nav-action-btn quick-tip"
          data-quick-tip="Please ensure current data is backed up first"
          :disabled="importingDatabase"
          @click="triggerDatabaseImport"
        >
          {{ importingDatabase ? 'Importing...' : 'Import' }}
        </button>
        <input
          ref="dbFileInput"
          type="file"
          accept=".db"
          style="display: none"
          @change="handleDatabaseFileSelected"
        />
        <hr class="nav-sep" />
        <button
          type="button"
          class="nav-link nav-action-btn quick-tip"
          data-quick-tip="Download database file. Contains all data except for uploaded files (documents)"
          :disabled="downloadingDatabaseBackup"
          @click="downloadDatabaseBackup"
        >
          <span class="nav-action-content">
            <svg class="nav-action-icon-svg" viewBox="0 0 24 24" aria-hidden="true">
              <ellipse
                cx="12"
                cy="5"
                rx="7"
                ry="2.5"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
              />
              <path
                d="M5 5v10c0 1.38 3.13 2.5 7 2.5s7-1.12 7-2.5V5"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
              />
              <path
                d="M5 10c0 1.38 3.13 2.5 7 2.5s7-1.12 7-2.5"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
              />
              <path
                d="M5 15c0 1.38 3.13 2.5 7 2.5s7-1.12 7-2.5"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
              />
            </svg>
            <span>{{
              downloadingDatabaseBackup ? 'Database backup in progress...' : 'Database Backup'
            }}</span>
          </span>
        </button>
        <button
          type="button"
          class="nav-link nav-action-btn quick-tip"
          data-quick-tip="Download full project backup. Excludes node_modules and dist directories"
          :disabled="downloadingFullBackup"
          @click="downloadFullBackup"
        >
          <span class="nav-action-content">
            <svg class="nav-action-icon-svg" viewBox="0 0 24 24" aria-hidden="true">
              <path
                d="M4 8.5 12 4l8 4.5-8 4.5L4 8.5Z"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linejoin="round"
              />
              <path
                d="M4 8.5V16l8 4 8-4V8.5"
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                stroke-linejoin="round"
              />
              <path d="M12 13v7" fill="none" stroke="currentColor" stroke-width="1.8" />
            </svg>
            <span>{{ downloadingFullBackup ? 'Full backup in progress...' : 'Full Backup' }}</span>
          </span>
        </button>

        <section v-if="recentViewHistory.length > 0" class="sidebar-recent-history">
          <div
            class="sidebar-recent-history-title sidebar-recent-history-title-tip"
            :data-quick-tip="recentHistoryQuickTip"
          >
            Recent History
          </div>
          <div class="sidebar-recent-history-list">
            <RouterLink
              v-for="entry in recentViewHistory"
              :key="`recent-history-${entry.id}`"
              :to="entry.url"
              class="sidebar-recent-history-link"
            >
              {{ formatRecentViewHistory(entry) }}
            </RouterLink>
          </div>
        </section>
      </nav>

      <div class="sidebar-footer">
        <div class="user-pill">
          <span class="mono">{{ auth.user?.username }}</span>
          <small>{{ auth.isReadOnly ? 'Read-only Permission' : 'Full Permission' }}</small>
        </div>
        <button class="ghost-btn" @click="logout">Logout</button>
      </div>
    </aside>

    <main class="app-main">
      <RouterView :key="routeKey" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import api from '../api/client';
import { useAuthStore } from '../stores/auth';
import { useNoticeStore } from '../stores/notice';

const auth = useAuthStore();
const noticeStore = useNoticeStore();
const saving = ref(false);
const testing = ref(false);
const loading = ref(false);
const error = ref('');
const savedSignature = ref('');

const form = reactive({
  useLdap: 0,
  ldapServer: '',
  ldapDn: '',
  ldapBindDn: '',
  ldapBindPassword: '',
  ldapGetUsers: '',
  ldapGetUsersFilter: '',
});

const ldapEnabled = computed(() => Number(form.useLdap ?? 0) === 1);

function buildSettingsSignature() {
  return JSON.stringify({
    useLdap: Number(form.useLdap ?? 0),
    ldapServer: form.ldapServer ?? '',
    ldapDn: form.ldapDn ?? '',
    ldapBindDn: form.ldapBindDn ?? '',
    ldapBindPassword: form.ldapBindPassword ?? '',
    ldapGetUsers: form.ldapGetUsers ?? '',
    ldapGetUsersFilter: form.ldapGetUsersFilter ?? '',
  });
}

function getRequiredFieldsError() {
  if (Number(form.useLdap ?? 0) !== 1) return '';

  const missing: string[] = [];
  if (!String(form.ldapServer ?? '').trim()) missing.push('Server address');
  if (!String(form.ldapDn ?? '').trim()) missing.push('Base DN');
  if (!String(form.ldapBindDn ?? '').trim()) missing.push('Bind DN');
  if (!String(form.ldapBindPassword ?? '').trim()) missing.push('Bind password');

  return missing.length > 0 ? `Please complete the required fields: ${missing.join(', ')}` : '';
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const { data } = await api.get('/settings');
    form.useLdap = Number(data.useldap ?? 0);
    form.ldapServer = data.ldap_server ?? '';
    form.ldapDn = data.ldap_dn ?? '';
    form.ldapBindDn = data.ldap_bind_dn ?? '';
    form.ldapBindPassword = data.ldap_bind_password ?? '';
    form.ldapGetUsers = data.ldap_getusers ?? '';
    form.ldapGetUsersFilter = data.ldap_getusers_filter ?? '';
    savedSignature.value = buildSettingsSignature();
  } catch (err: unknown) {
    error.value =
      (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
      'Failed to load system configuration';
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (auth.isReadOnly) return;

  const requiredError = getRequiredFieldsError();
  if (requiredError) {
    noticeStore.error(requiredError);
    return;
  }

  saving.value = true;
  error.value = '';
  try {
    const { data } = await api.put('/settings', { ...form });
    form.useLdap = Number(data.useldap ?? form.useLdap ?? 0);
    form.ldapServer = data.ldap_server ?? form.ldapServer;
    form.ldapDn = data.ldap_dn ?? form.ldapDn;
    form.ldapBindDn = data.ldap_bind_dn ?? form.ldapBindDn;
    form.ldapBindPassword = data.ldap_bind_password ?? form.ldapBindPassword;
    form.ldapGetUsers = data.ldap_getusers ?? form.ldapGetUsers;
    form.ldapGetUsersFilter = data.ldap_getusers_filter ?? form.ldapGetUsersFilter;
    savedSignature.value = buildSettingsSignature();
    noticeStore.success('Settings saved');
  } catch {
  } finally {
    saving.value = false;
  }
}

async function testConnection() {
  if (auth.isReadOnly) return;
  if (savedSignature.value === '' || savedSignature.value !== buildSettingsSignature()) {
    noticeStore.error('Please save the current LDAP configuration before testing the connection');
    return;
  }

  testing.value = true;
  error.value = '';
  try {
    const { data } = await api.post('/settings/test-ldap', { ...form });
    noticeStore.success(data?.message ?? 'LDAP connection successful');
  } catch {
  } finally {
    testing.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section class="page-shell">
    <header class="page-header">
      <h2>Settings</h2>
      <button class="ghost-btn" @click="load">Reload</button>
    </header>

    <p v-if="loading">Loading...</p>
    <p v-else-if="error" class="error-text section-gap">{{ error }}</p>

    <form v-else class="settings-grid settings-form" @submit.prevent="save">
      <div class="settings-ldap-intro">
        <h3>LDAP Configuration</h3>
        <p class="muted-text">
          Connection testing must be performed based on the saved configuration. The test only
          verifies the LDAP server connectivity and bind authentication, and does not actually
          execute user searches.
        </p>
        <p class="muted-text settings-ldap-note">
          %{attr} represents the LDAP attribute name participating in matching during login, and
          %{user} represents the currently entered username; these placeholders are retained for use
          as search filter templates.
        </p>
      </div>
      <label class="settings-field">
        <span class="settings-field-label">Enable LDAP</span>
        <select v-model.number="form.useLdap">
          <option :value="0">Disable</option>
          <option :value="1">Enable</option>
        </select>
      </label>
      <label class="settings-field">
        <span class="settings-field-label">Server Address</span>
        <input
          v-model="form.ldapServer"
          :disabled="!ldapEnabled"
          type="text"
          placeholder="For example: ldap://ad.example.com:389 or ldaps://ad.example.com:636"
        />
      </label>
      <label class="settings-field">
        <span class="settings-field-label">Base DN</span>
        <input
          v-model="form.ldapDn"
          :disabled="!ldapEnabled"
          type="text"
          placeholder="For example: OU=Users,DC=example,DC=com"
        />
      </label>
      <label class="settings-field">
        <span class="settings-field-label">Binding DN</span>
        <input
          v-model="form.ldapBindDn"
          :disabled="!ldapEnabled"
          type="text"
          placeholder="For example: CN=ldap-reader,OU=Service Accounts,DC=example,DC=com"
        />
      </label>
      <label class="settings-field">
        <span class="settings-field-label">Binding Password</span>
        <input
          v-model="form.ldapBindPassword"
          :disabled="!ldapEnabled"
          type="password"
          autocomplete="new-password"
          placeholder="Please enter the password for the binding DN"
        />
      </label>
      <label class="settings-field">
        <span class="settings-field-label">User Query Filter Template</span>
        <textarea
          v-model="form.ldapGetUsers"
          :disabled="!ldapEnabled"
          rows="3"
          placeholder="For example: (&(objectClass=user)(|(cn=%s)(sAMAccountName=%s)(userPrincipalName=%s)))"
        />
      </label>
      <label class="settings-field">
        <span class="settings-field-label">Additional Search Filter</span>
        <textarea
          v-model="form.ldapGetUsersFilter"
          :disabled="!ldapEnabled"
          rows="3"
          placeholder="For example: (%{attr}=%{user}) or (memberOf=CN=IT,OU=Groups,DC=example,DC=com)"
        />
        <small class="settings-field-help"
          >You can directly fill in templates like (%{attr}=%{user}); where %{attr} is the attribute
          name placeholder and %{user} is the username placeholder.</small
        >
      </label>

      <div class="settings-actions">
        <button :disabled="testing || saving || auth.isReadOnly" type="submit">
          {{ saving ? 'Saving...' : auth.isReadOnly ? 'Read-only Mode' : 'Save Settings' }}
        </button>
        <button
          :disabled="!ldapEnabled || testing || saving || auth.isReadOnly"
          class="ghost-btn"
          type="button"
          @click="testConnection"
        >
          {{ testing ? 'Testing...' : auth.isReadOnly ? 'Read-only Mode' : 'Test Connection' }}
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.settings-grid {
  max-width: 960px;
}

.settings-form {
  grid-template-columns: minmax(0, 1fr);
  gap: 14px;
}

.settings-ldap-intro {
  padding: 18px 20px;
  border: 1px solid rgba(47, 127, 186, 0.14);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(47, 127, 186, 0.07), rgba(47, 127, 186, 0.02));
}

.settings-ldap-intro h3 {
  margin: 0 0 6px;
}

.settings-ldap-note {
  margin-top: 6px;
}

.settings-field {
  gap: 8px;
}

.settings-field-label {
  color: #16324f;
}

.settings-field-help {
  color: #64748b;
  font-size: 0.85rem;
  line-height: 1.45;
}

.settings-form :deep(input),
.settings-form :deep(select),
.settings-form :deep(textarea) {
  min-height: 44px;
  padding: 0.65rem 0.8rem;
}

.settings-form :deep(input:disabled),
.settings-form :deep(select:disabled),
.settings-form :deep(textarea:disabled) {
  color: #7b8da0;
  background: #eef3f7;
  border-color: #d4dee8;
  cursor: not-allowed;
}

.settings-grid textarea {
  min-height: 92px;
  resize: vertical;
}

.settings-actions {
  display: flex;
  justify-content: flex-start;
  padding-top: 4px;
  gap: 10px;
  flex-wrap: wrap;
}

.settings-actions button {
  min-width: 120px;
  min-height: 44px;
}
</style>

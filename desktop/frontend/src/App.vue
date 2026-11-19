<script setup lang="ts">
import { computed, onMounted } from 'vue';
import AppLayout from './components/layout/AppLayout.vue';
import PageTitle from './components/common/PageTitle.vue';
import SitesView from './views/sites/index.vue';
import CacheView from './views/cache/index.vue';
import CertsView from './views/certs/index.vue';
import LogsView from './views/logs/index.vue';
import OperationsView from './views/operations/index.vue';
import AuditView from './views/audit/index.vue';
import MonitorView from './views/monitor/index.vue';
import SettingsView from './views/settings/index.vue';
import { useAppStore, type PaneKey } from './stores/app';
import { useI18n } from './composables/useI18n';

const app = useAppStore();
const { t } = useI18n();

const PANE_TITLES: Record<PaneKey, string> = {
  monitor: 'monitor.center', sites: 'nav.sites', cache: 'nav.cache',
  certs: 'nav.certs', logs: 'nav.logs',
  operations: 'nav.operations',
  audit: 'nav.audit',
  settings: 'settings.configTitle',
};
const paneTitle = computed(() => t(PANE_TITLES[app.pane]));

// 轮询移到 AppLayout：底部状态栏在所有页面都要显示实时状态。

onMounted(() => {
  // Frameless 窗口：F5 / Ctrl+R 强制重载；禁用浏览器右键菜单（输入框保留原生菜单）。
  window.addEventListener('keydown', (e: KeyboardEvent) => {
    if (e.key === 'F5' || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'r')) {
      e.preventDefault();
      window.location.reload();
    }
  });
  document.addEventListener('contextmenu', (e: MouseEvent) => {
    const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
    if (tag !== 'input' && tag !== 'textarea') e.preventDefault();
  });
});
</script>

<template>
  <AppLayout>
    <!-- 站点 / 缓存区 / 证书自带标题栏：主操作按钮要放在标题右侧。 -->
    <PageTitle
      v-if="app.pane !== 'monitor' && app.pane !== 'sites' && app.pane !== 'cache' && app.pane !== 'certs'"
      :title="paneTitle"
    />
    <MonitorView v-if="app.pane === 'monitor'" />
    <SitesView v-else-if="app.pane === 'sites'" />
    <CacheView v-else-if="app.pane === 'cache'" />
    <CertsView v-else-if="app.pane === 'certs'" />
    <LogsView v-else-if="app.pane === 'logs'" />
    <OperationsView v-else-if="app.pane === 'operations'" />
    <AuditView v-else-if="app.pane === 'audit'" />
    <SettingsView v-else-if="app.pane === 'settings'" />
    <div v-else class="main__body">
      <section class="placeholder">
        <p>{{ paneTitle }}</p>
        <small>{{ t('nav.group') }}</small>
      </section>
    </div>
  </AppLayout>
</template>

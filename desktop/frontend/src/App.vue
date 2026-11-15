<script setup lang="ts">
import { computed, onMounted } from 'vue';
import AppLayout from './components/layout/AppLayout.vue';
import AppHeader from './components/common/AppHeader.vue';
import SitesView from './views/sites/index.vue';
import CertsView from './views/certs/index.vue';
import LogsView from './views/logs/index.vue';
import OperationsView from './views/operations/index.vue';
import VersionsView from './views/versions/index.vue';
import MonitorView from './views/monitor/index.vue';
import SettingsView from './views/settings/index.vue';
import { useAppStore, type PaneKey } from './stores/app';
import { useOverviewStore } from './stores/overview';
import { useI18n } from './composables/useI18n';

const app = useAppStore();
const overview = useOverviewStore();
const { t } = useI18n();

const PANE_TITLES: Record<PaneKey, string> = {
  monitor: 'monitor.center', sites: 'nav.sites',
  certs: 'nav.certs', logs: 'nav.logs',
  operations: 'nav.operations', versions: 'settings.versionsTitle',
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
    <AppHeader :title="paneTitle" :running="overview.data?.running ?? false" />
    <MonitorView v-if="app.pane === 'monitor'" />
    <SitesView v-else-if="app.pane === 'sites'" />
    <CertsView v-else-if="app.pane === 'certs'" />
    <LogsView v-else-if="app.pane === 'logs'" />
    <OperationsView v-else-if="app.pane === 'operations'" />
    <VersionsView v-else-if="app.pane === 'versions'" />
    <SettingsView v-else-if="app.pane === 'settings'" />
    <div v-else class="main__body">
      <section class="placeholder">
        <p>{{ paneTitle }}</p>
        <small>{{ t('nav.group') }}</small>
      </section>
    </div>
  </AppLayout>
</template>

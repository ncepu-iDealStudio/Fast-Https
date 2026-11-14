<script setup lang="ts">
import { computed, onMounted } from 'vue';
import AppLayout from './components/layout/AppLayout.vue';
import AppHeader from './components/common/AppHeader.vue';
import OverviewView from './views/overview/index.vue';
import MatrixView from './views/matrix/index.vue';
import SitesView from './views/sites/index.vue';
import CertsView from './views/certs/index.vue';
import LogsView from './views/logs/index.vue';
import RuntimeView from './views/runtime/index.vue';
import OperationsView from './views/operations/index.vue';
import SettingsView from './views/settings/index.vue';
import { useAppStore, type PaneKey } from './stores/app';
import { useOverviewStore } from './stores/overview';
import { useI18n } from './composables/useI18n';
import { usePolling } from './composables/usePolling';

const app = useAppStore();
const overview = useOverviewStore();
const { t } = useI18n();

const PANE_TITLES: Record<PaneKey, string> = {
  overview: 'nav.overview', matrix: 'nav.matrix', sites: 'nav.sites',
  certs: 'nav.certs', logs: 'nav.logs', runtime: 'nav.runtime',
  operations: 'nav.operations', settings: 'settings.configTitle',
};
const paneTitle = computed(() => t(PANE_TITLES[app.pane]));

// 只有停在概览页时才轮询，切走自动暂停（与原版一致）。
const overviewActive = computed(() => app.pane === 'overview');
usePolling(() => { void overview.refresh(); }, 2000, overviewActive);

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
    <OverviewView v-if="app.pane === 'overview'" />
    <MatrixView v-else-if="app.pane === 'matrix'" />
    <SitesView v-else-if="app.pane === 'sites'" />
    <CertsView v-else-if="app.pane === 'certs'" />
    <LogsView v-else-if="app.pane === 'logs'" />
    <RuntimeView v-else-if="app.pane === 'runtime'" />
    <OperationsView v-else-if="app.pane === 'operations'" />
    <SettingsView v-else-if="app.pane === 'settings'" />
    <div v-else class="main__body">
      <section class="placeholder">
        <p>{{ paneTitle }}</p>
        <small>{{ t('nav.group') }}</small>
      </section>
    </div>
  </AppLayout>
</template>

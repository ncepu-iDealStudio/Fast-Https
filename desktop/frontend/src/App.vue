<script setup lang="ts">
import { computed, onMounted } from 'vue';
import AppLayout from './components/layout/AppLayout.vue';
import AppHeader from './components/common/AppHeader.vue';
import OverviewView from './views/overview/index.vue';
import { useAppStore } from './stores/app';
import { useOverviewStore } from './stores/overview';
import { useI18n } from './composables/useI18n';
import { usePolling } from './composables/usePolling';

const app = useAppStore();
const overview = useOverviewStore();
const { t } = useI18n();

const paneTitle = computed(() => {
  const keys: Record<string, string> = {
    overview: 'nav.overview', matrix: 'nav.matrix', sites: 'nav.sites',
    certs: 'nav.certs', logs: 'nav.logs', runtime: 'nav.runtime',
    operations: 'nav.operations', settings: 'settings.configTitle',
  };
  return t(keys[app.pane] ?? 'nav.overview');
});

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
    <OverviewView v-if="overviewActive" />
    <div v-else class="main__body">
      <section class="placeholder">
        <p>{{ paneTitle }}</p>
        <small>{{ t('nav.group') }} · {{ paneTitle }}</small>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { Activity, Globe, ShieldCheck, ScrollText, History, GitBranch } from '@lucide/vue';
import type { Component } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore, type PaneKey } from '../../stores/app';
import AccountMenu from '../common/AccountMenu.vue';

const { t } = useI18n();
const app = useAppStore();

// 与旧版一致：导航只有 7 项，设置从底部账户菜单进入，不占导航位。
interface NavItem { key: PaneKey; label: string; icon: Component }
const items: NavItem[] = [
  { key: 'monitor', label: 'monitor.center', icon: Activity },
  { key: 'sites', label: 'nav.sites', icon: Globe },
  { key: 'certs', label: 'nav.certs', icon: ShieldCheck },
  { key: 'logs', label: 'nav.logs', icon: ScrollText },
  { key: 'operations', label: 'nav.operations', icon: History },
  { key: 'versions', label: 'settings.versionsTitle', icon: GitBranch },
];
</script>

<template>
  <aside class="sidebar">
    <div class="group">{{ t('nav.group') }}</div>
    <button
      v-for="item in items" :key="item.key"
      class="nav-item" :class="{ on: app.pane === item.key }"
      :title="t(item.label)"
      @click="app.activate(item.key)"
    >
      <component :is="item.icon" :size="16" />
      <span>{{ t(item.label) }}</span>
    </button>

    <!-- 底部账户区：主题/语言切换、设置、隐藏到托盘、帮助 -->
    <AccountMenu />
  </aside>
</template>

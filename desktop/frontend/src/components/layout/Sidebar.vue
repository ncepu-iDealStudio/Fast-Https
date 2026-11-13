<script setup lang="ts">
import { Activity, LayoutGrid, Globe, ShieldCheck, ScrollText, Zap, History, Settings2, ChevronDown } from '@lucide/vue';
import type { Component } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore, type PaneKey } from '../../stores/app';

const { t } = useI18n();
const app = useAppStore();

// 与旧版一致：导航只有 7 项，设置从底部账户菜单进入，不占导航位。
interface NavItem { key: PaneKey; label: string; icon: Component }
const items: NavItem[] = [
  { key: 'overview', label: 'nav.overview', icon: Activity },
  { key: 'matrix', label: 'nav.matrix', icon: LayoutGrid },
  { key: 'sites', label: 'nav.sites', icon: Globe },
  { key: 'certs', label: 'nav.certs', icon: ShieldCheck },
  { key: 'logs', label: 'nav.logs', icon: ScrollText },
  { key: 'runtime', label: 'nav.runtime', icon: Zap },
  { key: 'operations', label: 'nav.operations', icon: History },
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

    <!-- 底部账户/设置区，沿用旧版 .side-foot 布局 -->
    <div class="side-foot-wrap">
      <button class="side-foot" type="button" @click="app.activate('settings')">
        <span class="avatar"><Settings2 :size="12" /></span>
        <span class="acct-name">{{ t('nav.account') }}</span>
        <ChevronDown :size="12" class="acct-caret" />
      </button>
    </div>
  </aside>
</template>

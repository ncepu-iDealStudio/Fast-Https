<script setup lang="ts">
import { Activity, LayoutGrid, Globe, ShieldCheck, ScrollText, Zap, History, Settings2 } from '@lucide/vue';
import type { Component } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore, type PaneKey } from '../../stores/app';

const { t } = useI18n();
const app = useAppStore();

interface NavItem { key: PaneKey; label: string; icon: Component }
const items: NavItem[] = [
  { key: 'overview', label: 'nav.overview', icon: Activity },
  { key: 'matrix', label: 'nav.matrix', icon: LayoutGrid },
  { key: 'sites', label: 'nav.sites', icon: Globe },
  { key: 'certs', label: 'nav.certs', icon: ShieldCheck },
  { key: 'logs', label: 'nav.logs', icon: ScrollText },
  { key: 'runtime', label: 'nav.runtime', icon: Zap },
  { key: 'operations', label: 'nav.operations', icon: History },
  { key: 'settings', label: 'settings.configTitle', icon: Settings2 },
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
  </aside>
</template>

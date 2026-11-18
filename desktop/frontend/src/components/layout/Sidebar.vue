<script setup lang="ts">
/** 左侧导航：可折叠为纯图标模式；底部账户区已移到底部横带，不在此处。 */
import { Activity, Globe, Database, ShieldCheck, ScrollText, History, GitBranch, ChevronsLeft, ChevronsRight } from '@lucide/vue';
import type { Component } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore, type PaneKey } from '../../stores/app';

const { t } = useI18n();
const app = useAppStore();

interface NavItem { key: PaneKey; label: string; icon: Component }
const items: NavItem[] = [
  { key: 'monitor', label: 'monitor.center', icon: Activity },
  { key: 'sites', label: 'nav.sites', icon: Globe },
  { key: 'cache', label: 'nav.cache', icon: Database },
  { key: 'certs', label: 'nav.certs', icon: ShieldCheck },
  { key: 'logs', label: 'nav.logs', icon: ScrollText },
  { key: 'operations', label: 'nav.operations', icon: History },
  { key: 'versions', label: 'settings.versionsTitle', icon: GitBranch },
];
</script>

<template>
  <aside class="sidebar" :class="{ 'sidebar--rail': app.railCollapsed }">
    <div class="sidebar__head">
      <span v-if="!app.railCollapsed" class="group">{{ t('nav.group') }}</span>
      <button
        class="rail-toggle" :title="app.railCollapsed ? t('nav.expand') : t('nav.collapse')"
        @click="app.toggleRail()"
      >
        <component :is="app.railCollapsed ? ChevronsRight : ChevronsLeft" :size="14" />
      </button>
    </div>

    <nav class="sidebar__nav">
      <button
        v-for="item in items" :key="item.key"
        class="nav-item" :class="{ on: app.pane === item.key }"
        :title="app.railCollapsed ? t(item.label) : undefined"
        @click="app.activate(item.key)"
      >
        <component :is="item.icon" :size="16" class="nav-item__ico" />
        <span v-if="!app.railCollapsed" class="nav-item__txt">{{ t(item.label) }}</span>
      </button>
    </nav>
  </aside>
</template>

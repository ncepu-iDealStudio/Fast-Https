<script setup lang="ts">
/**
 * 左侧悬浮导航坞：可折叠为纯图标模式。
 * 底部「脉搏卡」用心电图动画表示服务是否在跳动——停止时变成一条平线。
 */
import { computed } from 'vue';
import { ChevronsLeft, ChevronsRight } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore } from '../../stores/app';
import { useOverviewStore } from '../../stores/overview';
import { NAV_ITEMS } from './navItems';

const { t } = useI18n();
const app = useAppStore();
const overview = useOverviewStore();

const running = computed(() => !!overview.data?.running);
const activeIndex = computed(() => NAV_ITEMS.findIndex((i) => i.key === app.pane));
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

    <nav class="sidebar__nav" :style="{ '--active-i': activeIndex }">
      <!-- 滑动高亮块：跟随选中项平移，而不是每项各自闪现 -->
      <span v-if="activeIndex >= 0" class="nav-glider" aria-hidden="true" />
      <button
        v-for="(item, i) in NAV_ITEMS" :key="item.key"
        class="nav-item" :class="{ on: app.pane === item.key }"
        :style="{ '--i': i }"
        :title="app.railCollapsed ? t(item.label) : undefined"
        @click="app.activate(item.key)"
      >
        <span class="nav-item__tile"><component :is="item.icon" :size="15" class="nav-item__ico" /></span>
        <span v-if="!app.railCollapsed" class="nav-item__txt">{{ t(item.label) }}</span>
      </button>
    </nav>

    <div class="pulse-card" :class="{ 'pulse-card--on': running }" :title="running ? t('status.running') : t('status.stopped')">
      <svg class="pulse-card__ecg" viewBox="0 0 120 32" preserveAspectRatio="none" aria-hidden="true">
        <path
          v-if="running"
          d="M0 16 H34 L40 16 L44 6 L50 27 L55 10 L59 16 H78 L82 12 L86 16 H120"
        />
        <path v-else d="M0 16 H120" />
      </svg>
      <div v-if="!app.railCollapsed" class="pulse-card__text">
        <span class="pulse-card__dot" />
        <b>{{ running ? t('status.running') : t('status.stopped') }}</b>
        <small v-if="running && overview.data?.pid" class="mono">pid {{ overview.data.pid }}</small>
      </div>
      <span v-else class="pulse-card__dot pulse-card__dot--solo" />
    </div>
  </aside>
</template>

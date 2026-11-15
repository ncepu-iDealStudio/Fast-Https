<script setup lang="ts">
/**
 * 窗口骨架：顶栏 + 侧栏 + 主区 + 底部状态栏。
 * 状态栏常驻所有页面：左段本机管理（宽=侧栏宽）、右段运行状态。
 */
import { computed, onMounted, ref } from 'vue';
import TitleBar from './TitleBar.vue';
import Sidebar from './Sidebar.vue';
import AccountMenu from '../common/AccountMenu.vue';
import { useAppStore } from '../../stores/app';
import { useOverviewStore } from '../../stores/overview';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
import { startServer, stopServer } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import { Play, Square, RotateCw } from '@lucide/vue';

const app = useAppStore();
const overview = useOverviewStore();
const { t } = useI18n();

const busy = ref(false);
const footError = ref('');

// 状态栏轮询：全局常开 + 静默（不碰 loading），2 秒更新一次底部状态。
// 手动按钮不会因为轮询闪 spinner。
usePolling(() => { void overview.refresh(true); }, 2000, computed(() => true));

// 启动时长：不足 1 分钟不显示 0 分钟。
const uptimeText = computed(() => {
  const s = Number(overview.data?.uptimeSeconds ?? 0);
  if (s <= 0) return '';
  const m = Math.floor(s / 60);
  return m > 0 ? t('status.uptimeMinutes', { m }) : t('status.uptimeLess');
});

async function toggleServer(): Promise<void> {
  busy.value = true;
  footError.value = '';
  try {
    if (overview.data?.running) await stopServer();
    else await startServer();
    await overview.refresh(true);
  } catch (e) {
    footError.value = messageOf(e);
  } finally {
    busy.value = false;
  }
}

onMounted(() => { void overview.refresh(true); });
</script>

<template>
  <div class="window" :class="{ 'window--rail': app.railCollapsed }">
    <TitleBar />
    <div class="app">
      <Sidebar />
      <main class="main">
        <slot />
      </main>
    </div>

    <!-- 底部横带：左段本机管理（宽=侧栏宽），右段状态信息 -->
    <footer class="footbar">
      <AccountMenu class="footbar__account" />

      <div class="footbar__status">
        <span class="dot" :class="overview.data?.running ? 'state-ok' : 'state-off'" />
        <strong>{{ overview.data?.running ? t('status.running') : t('status.stopped') }}</strong>
        <template v-if="overview.data?.running">
          <span class="footbar__sep">·</span>
          <span class="mono">pid {{ overview.data?.pid ?? '—' }}</span>
          <span v-if="uptimeText" class="footbar__sep">·</span>
          <span v-if="uptimeText">{{ uptimeText }}</span>
        </template>
        <template v-else>
          <span class="footbar__sep">·</span>
          <span>{{ t('status.subStopped') }}</span>
        </template>
        <span class="footbar__sep">·</span>
        <span class="footbar__hint">{{ t('status.refreshing') }}</span>
        <span v-if="footError" class="footbar__err">{{ footError }}</span>
        <span class="footbar__spacer" />
        <span class="footbar__checked mono">{{ overview.data?.checkedAt || '' }}</span>
        <button
          class="fbtn" :title="overview.data?.running ? t('overview.stop') : t('overview.start')"
          :disabled="busy" @click="toggleServer"
        >
          <component :is="overview.data?.running ? Square : Play" :size="12" />
        </button>
        <button class="fbtn" :title="t('common.refresh')" @click="overview.refresh(true)">
          <RotateCw :size="12" />
        </button>
      </div>
    </footer>
  </div>
</template>
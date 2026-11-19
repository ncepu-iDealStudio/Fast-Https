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
import { startServer, stopServer, openRepo } from '../../services/api/app';
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

// 状态栏的 GitHub 入口：只打开系统浏览器，失败时复用状态栏的错误位。
async function starRepo(): Promise<void> {
  footError.value = '';
  try {
    await openRepo();
  } catch (e) {
    footError.value = messageOf(e);
  }
}
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
        <!-- GitHub 品牌图标：lucide v1 已移除品牌图标，这里用官方 mark 的内联 SVG。 -->
        <button class="fbtn fbtn--github" :title="t('status.star')" @click="starRepo">
          <svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor" aria-hidden="true">
            <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
          </svg>
        </button>
      </div>
    </footer>
  </div>
</template>
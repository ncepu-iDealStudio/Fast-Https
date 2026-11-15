<script setup lang="ts">
/**
 * 窗口骨架：顶栏 + 侧栏 + 主区 + 底部状态栏。
 * 状态栏常驻所有页面：状态点 / 运行状态 / pid / 端口 / 刷新时刻在左，
 * 服务启停与健康检查在右——启停是全局操作，不该只在概览页可用。
 */
import { computed, onMounted, ref } from 'vue';
import TitleBar from './TitleBar.vue';
import Sidebar from './Sidebar.vue';
import { useOverviewStore } from '../../stores/overview';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
import { startServer, stopServer } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import { Play, Square, RotateCw } from '@lucide/vue';

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
  <div class="window">
    <TitleBar />
    <div class="app">
      <Sidebar />
      <main class="main">
        <slot />
      </main>
    </div>

    <footer class="footbar">
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
      </div>

      <div class="footbar__actions">
        <span class="footbar__checked mono">{{ overview.data?.checkedAt || '' }}</span>
        <button
          class="icon-btn" :title="overview.data?.running ? t('overview.stop') : t('overview.start')"
          :disabled="busy" @click="toggleServer"
        >
          <component :is="overview.data?.running ? Square : Play" :size="14" />
        </button>
        <button class="icon-btn" :title="t('common.refresh')" @click="overview.refresh(true)">
          <RotateCw :size="14" />
        </button>
      </div>
    </footer>
  </div>
</template>

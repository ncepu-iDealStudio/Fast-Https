<script setup lang="ts">
/**
 * 无声偶遇低语（PhilosophyWhisper）：
 * 不需要用户主动点击，仅在以下自然契机悄然浮现 4.5 秒后消散，且完全穿透点击不打扰操作：
 *  1. 服务真实从停止→启动，或启动→停止的瞬间；
 *  2. 撞见对称时刻（如 11:11 / 22:22 / 12:21）或整点 / 零点；
 *  3. 深夜（23:00–04:59）首次停留满 20 秒的守望问候。
 */
import { onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { useOverviewStore } from '../../stores/overview';
import { lateNightWhisper, specialClockWhisper, transitionWhisper } from '../../content/philosophy';

const NIGHT_KEY = 'fast-https.nightWhispered';
const SHOW_MS = 4600;

const { lang } = useI18n();
const overview = useOverviewStore();

const text = ref('');
const visible = ref(false);

let hideTimer: number | undefined;
let clockTimer: number | undefined;
let nightTimer: number | undefined;
let transitionCount = 0;
let lastKnownRunning: boolean | null = null;
const seenSpecial = new Set<string>();

function reveal(msg: string): void {
  if (!msg) return;
  text.value = msg;
  visible.value = true;
  if (hideTimer) window.clearTimeout(hideTimer);
  hideTimer = window.setTimeout(() => {
    visible.value = false;
  }, SHOW_MS);
}

// 1) 服务真实启停瞬间（忽略首次加载时的初始态）
watch(
  () => overview.data?.running,
  (val) => {
    if (typeof val !== 'boolean') return;
    if (lastKnownRunning === null) {
      lastKnownRunning = val;
      return;
    }
    if (val !== lastKnownRunning) {
      lastKnownRunning = val;
      transitionCount += 1;
      reveal(transitionWhisper(lang(), val, transitionCount));
    }
  },
);

function checkClock(): void {
  const hit = specialClockWhisper(lang(), new Date());
  if (hit && !seenSpecial.has(hit.key)) {
    seenSpecial.add(hit.key);
    reveal(hit.text);
  }
}

onMounted(() => {
  // 2) 每 5 秒静默检查一次是否撞见对称/整点时刻
  clockTimer = window.setInterval(checkClock, 5000);

  // 3) 深夜首次停留 20 秒后的温柔问候（每会话最多一次）
  const h = new Date().getHours();
  if (h >= 23 || h < 5) {
    let seen = false;
    try { seen = sessionStorage.getItem(NIGHT_KEY) === '1'; } catch { /* ignore */ }
    if (!seen) {
      nightTimer = window.setTimeout(() => {
        try { sessionStorage.setItem(NIGHT_KEY, '1'); } catch { /* ignore */ }
        if (!visible.value) reveal(lateNightWhisper(lang()));
      }, 20000);
    }
  }
});

onUnmounted(() => {
  if (hideTimer) window.clearTimeout(hideTimer);
  if (clockTimer) window.clearInterval(clockTimer);
  if (nightTimer) window.clearTimeout(nightTimer);
});
</script>

<template>
  <Transition name="whisper">
    <aside v-if="visible" class="whisper" aria-live="polite" aria-atomic="true">
      <span class="whisper__dot" aria-hidden="true" />
      <span class="whisper__txt">{{ text }}</span>
    </aside>
  </Transition>
</template>

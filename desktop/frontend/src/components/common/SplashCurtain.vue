<script setup lang="ts">
/**
 * 开屏片刻：冷启动时如薄雾般呈现约 2.2 秒的哲学短句，随后自然淡出。
 * 同一会话内 F5 刷新不反复打扰；轻触或按键可随时提前散开，不强迫交互。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from '../../composables/useI18n';
import { quoteForMoment, timeMoodOf } from '../../content/philosophy';

const SEEN_KEY = 'fast-https.splashSeen';
const DURATION_MS = 2200;

const { lang } = useI18n();
const visible = ref(false);

const mood = computed(() => timeMoodOf(lang()));
const quote = computed(() => quoteForMoment(lang()));

let timer: number | undefined;

function dismiss(): void {
  if (!visible.value) return;
  visible.value = false;
  if (timer) {
    window.clearTimeout(timer);
    timer = undefined;
  }
}

onMounted(() => {
  try {
    if (sessionStorage.getItem(SEEN_KEY) === '1') return;
    sessionStorage.setItem(SEEN_KEY, '1');
  } catch { /* ignore */ }

  visible.value = true;
  window.addEventListener('keydown', dismiss, { once: true });
  timer = window.setTimeout(dismiss, DURATION_MS);
});

onUnmounted(() => {
  window.removeEventListener('keydown', dismiss);
  if (timer) window.clearTimeout(timer);
});
</script>

<template>
  <Transition name="splash">
    <div v-if="visible" class="splash" role="status" aria-live="polite" @click="dismiss">
      <div class="splash__card">
        <svg class="splash__orbit" viewBox="0 0 64 64" aria-hidden="true">
          <circle cx="32" cy="32" r="24" class="orbit__ring" />
          <circle cx="32" cy="32" r="14" class="orbit__ring orbit__ring--inner" />
          <g class="orbit__sat">
            <circle cx="32" cy="8" r="2.5" class="orbit__dot" />
          </g>
          <circle cx="32" cy="32" r="2.8" class="orbit__core" />
        </svg>

        <div class="splash__mood">
          <span>{{ mood.greeting }}</span>
          <span class="splash__dot">·</span>
          <span>fast-https</span>
        </div>

        <p class="splash__quote">“{{ quote.text }}”</p>
        <div class="splash__author">—— {{ quote.author }}</div>

        <div class="splash__bar" aria-hidden="true">
          <span class="splash__bar-fill" />
        </div>
      </div>
    </div>
  </Transition>
</template>

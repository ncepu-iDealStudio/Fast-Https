<script setup lang="ts">
/** 顶部服务状态卡：状态大字 + LIVE 徽标 + 事实条 + 右侧操作。 */
import { computed } from 'vue';
import { Play, Square, RotateCw, Stethoscope } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

const props = defineProps<{
  running: boolean;
  pid?: number;
  uptime: string;
  ports: string;
  monitorHealthy: boolean;
  loading: boolean;
}>();
const emit = defineEmits<{
  toggle: []; reload: []; diagnose: []; refresh: [];
}>();
const { t } = useI18n();

const dash = '\u2014';
const facts = computed(() => [
  { label: 'PROCESS ID', value: props.pid != null ? String(props.pid) : dash },
  { label: 'UPTIME', value: props.uptime || dash },
  { label: 'LISTENING', value: props.ports || dash },
  { label: 'INSTANCE', value: t('nav.account') },
  { label: 'MONITOR', value: props.monitorHealthy ? 'Healthy' : dash, ok: props.monitorHealthy },
]);
</script>

<template>
  <section class="hero" :class="{ 'hero--off': !running }">
    <div class="hero__head">
      <div>
        <div class="hero__eyebrow">SERVICE STATUS</div>
        <h2 class="hero__title">
          {{ running ? t('status.running') : t('status.stopped') }}
          <span v-if="running" class="hero__live">LIVE</span>
        </h2>
        <p class="hero__sub">
          {{ running ? t('status.subRunning') : t('status.subStopped') }}
          <template v-if="running && monitorHealthy"> - Monitor {{ t('monitor.statusRunning') }}</template>
        </p>
      </div>
      <div class="hero__actions">
        <button class="hero__cta" :class="{ 'hero__cta--stop': running }" @click="emit('toggle')">
          <component :is="running ? Square : Play" :size="13" />
          <span>{{ running ? t('overview.stop') : t('overview.start') }}</span>
        </button>
        <button class="hero__cta" @click="emit('reload')"><RotateCw :size="13" /><span>{{ t('overview.reload') }}</span></button>
        <button class="hero__cta" @click="emit('diagnose')"><Stethoscope :size="13" /><span>{{ t('overview.diagnose') }}</span></button>
      </div>
    </div>
    <div class="hero__facts">
      <div v-for="f in facts" :key="f.label" class="hero__fact">
        <small>{{ f.label }}</small>
        <b class="mono" :class="{ 'is-ok': f.ok }">{{ f.value }}</b>
      </div>
    </div>
  </section>
</template>

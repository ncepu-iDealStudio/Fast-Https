<script setup lang="ts">
/** 顶部服务状态卡：状态 / LIVE 徽标 / 关键事实 / 右侧操作图标。 */
import { Play, Square, RotateCw, FolderOpen, Stethoscope, RefreshCw } from '@lucide/vue';
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
  toggle: [];
  reload: [];
  diagnose: [];
  openDir: [];
  refresh: [];
}>();
const { t } = useI18n();

const facts = () => [
  { label: 'PROCESS ID', value: props.pid != null ? String(props.pid) : '—' },
  { label: 'UPTIME', value: props.uptime || '—' },
  { label: 'LISTENING', value: props.ports || '—' },
  { label: 'INSTANCE', value: t('nav.account') },
  { label: 'MONITOR', value: props.monitorHealthy ? 'Healthy' : '—', ok: props.monitorHealthy },
];
</script>

<template>
  <section class="hero" :class="{ 'hero--off': !running }">
    <div class="hero__head">
      <div>
        <div class="hero__eyebrow">SERVICE CONTROL CENTER</div>
        <h2 class="hero__title">
          {{ running ? t('status.running') : t('status.stopped') }}
          <span v-if="running" class="hero__live">LIVE</span>
        </h2>
        <p class="hero__sub">{{ running ? t('status.subRunning') : t('status.subStopped') }}</p>
      </div>
      <div class="hero__actions">
        <button class="hero__btn" :title="running ? t('overview.stop') : t('overview.start')" @click="emit('toggle')">
          <component :is="running ? Square : Play" :size="14" />
        </button>
        <button class="hero__btn" :title="t('overview.reload')" @click="emit('reload')"><RotateCw :size="14" /></button>
        <button class="hero__btn" :title="t('overview.diagnose')" @click="emit('diagnose')"><Stethoscope :size="14" /></button>
        <button class="hero__btn" :title="t('overview.openConfig')" @click="emit('openDir')"><FolderOpen :size="14" /></button>
        <button class="hero__btn" :title="t('common.refresh')" :disabled="loading" @click="emit('refresh')"><RefreshCw :size="14" /></button>
      </div>
    </div>
    <div class="hero__facts">
      <div v-for="f in facts()" :key="f.label" class="hero__fact">
        <small>{{ f.label }}</small>
        <b class="mono" :class="{ 'is-ok': f.ok }">{{ f.value }}</b>
      </div>
    </div>
  </section>
</template>

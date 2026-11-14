<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { Play, Square, RotateCw, FolderOpen, ScrollText, Globe, ShieldCheck, Stethoscope } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore } from '../../stores/app';
import { useOverviewStore } from '../../stores/overview';
import {
  startServer, stopServer, reloadServer, openConfigDir,
} from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import FhButton from '../../components/ui/FhButton.vue';
import DiagnoseDialog from '../../components/common/DiagnoseDialog.vue';

const { t } = useI18n();
const app = useAppStore();
const ov = useOverviewStore();

const STATUS_TEXT: Record<string, string> = {
  running: 'status.running', degraded: 'status.degraded',
  stopped: 'status.stopped', unknown: 'status.unknown',
};
const STATUS_SUB: Record<string, string> = {
  running: 'status.subRunning', degraded: 'status.subDegraded',
  stopped: 'status.subStopped', unknown: 'status.unknown',
};
const STATUS_CLASS: Record<string, string> = {
  running: 'state-ok', degraded: 'state-warn', stopped: 'state-off', unknown: '',
};

const statusKey = computed(() => {
  const s = ov.data?.status ?? '';
  if (STATUS_TEXT[s]) return s;
  return ov.data?.running ? 'running' : 'stopped';
});
const statusText = computed(() => t(STATUS_TEXT[statusKey.value] ?? 'status.unknown'));
const statusSub = computed(() => t(STATUS_SUB[statusKey.value] ?? 'status.unknown'));
const statusClass = computed(() => STATUS_CLASS[statusKey.value] ?? '');

/** 运行 N 分钟：不足 1 分钟不显示 0 分钟。 */
const uptimeText = computed(() => {
  const s = Number(ov.data?.uptimeSeconds ?? 0);
  if (s <= 0) return '';
  const m = Math.floor(s / 60);
  return m > 0 ? t('status.uptimeMinutes', { m }) : t('status.uptimeLess');
});

const metrics = computed(() => [
  { key: 'overview.metricSites', value: ov.data?.siteCount ?? 0 },
  { key: 'overview.metricCerts', value: ov.data?.tlsCount ?? 0 },
  { key: 'overview.metricPorts', value: (ov.data?.ports ?? []).join(', ') || '-', mono: true },
]);

const hasAlert = computed(() => !!ov.data?.error || ov.data?.configDirty);

async function primary(): Promise<void> {
  try {
    if (ov.data?.running) await stopServer();
    else await startServer();
    await ov.refresh();
  } catch {
    await ov.refresh();
  }
}

async function reload(): Promise<void> {
  try { await reloadServer(); } finally { await ov.refresh(); }
}

async function openDir(): Promise<void> {
  const p = ov.data?.paths?.config;
  if (p) {
    try { await openConfigDir(p); } catch (e) { console.error(messageOf(e)); }
  }
}

const quick = [
  { key: 'overview.quickSites', icon: Globe, pane: 'sites' as const },
  { key: 'overview.quickCerts', icon: ShieldCheck, pane: 'certs' as const },
  { key: 'overview.quickLogs', icon: ScrollText, pane: 'logs' as const },
];

const diagnoseOpen = ref(false);

/**
 * 本机动态时间线：原版是在状态轮询时把变化合成到本地列表（最近 5 条），
 * 这里用同样的做法 —— 监听 overview 数据，状态或配置版本变化时记一条。
 */
interface Ev { time: string; text: string; kind: 'info' | 'warn' | 'ok' }
const events = ref<Ev[]>([]);
let lastStatus = '';

function nowTime(): string {
  const d = new Date();
  const p = (n: number): string => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
function pushEvent(text: string, kind: Ev['kind']): void {
  events.value.unshift({ time: nowTime(), text, kind });
  if (events.value.length > 5) events.value.pop();
}

watch(() => ov.data, (d) => {
  if (!d) return;
  const key = `${d.status}|${d.running}`;
  if (key !== lastStatus) {
    if (lastStatus === '') pushEvent(t('events.started'), 'info');
    else if (d.running) pushEvent(t('events.serverUp'), 'ok');
    else pushEvent(t('events.serverDown'), 'warn');
    lastStatus = key;
  }
  if (d.error) pushEvent(t('events.configError'), 'warn');
}, { immediate: true });
</script>

<template>
  <div class="main__body">
    <!-- 告警条：配置读取出错或配置版本不一致时提示 -->
    <div v-if="hasAlert" class="alert-strip">
      <span class="alert-strip__title">
        {{ ov.data?.error ? t('overview.errorTitle') : t('overview.alertDirtyTitle') }}
      </span>
      <span class="alert-strip__body">
        {{ ov.data?.error || t('overview.alertDirtyBody') }}
      </span>
    </div>

    <!-- 杩愯姒傚喌鍗?-->
    <div class="card overview-card">
      <div class="card-head">
        <h3>{{ t('overview.title') }}</h3>
        <span class="card-note">{{ ov.data?.checkedAt || '' }}</span>
      </div>
      <div class="overview-state">
        <span class="dot" :class="statusClass" />
        <b class="overview-state__text">{{ statusText }}</b>
        <span class="muted">{{ statusSub }}</span>
        <span v-if="uptimeText" class="muted">路 {{ uptimeText }}</span>
        <span v-if="ov.data?.pid" class="muted mono">路 pid {{ ov.data.pid }}</span>
      </div>
      <div class="overview-actions">
        <FhButton variant="primary" @click="primary">
          <component :is="ov.data?.running ? Square : Play" :size="14" />
          <span>{{ ov.data?.running ? t('overview.stop') : t('overview.start') }}</span>
        </FhButton>
        <FhButton :loading="ov.loading" @click="reload">
          <RotateCw :size="14" /><span>{{ t('overview.reload') }}</span>
        </FhButton>
        <FhButton @click="openDir">
          <FolderOpen :size="14" /><span>{{ t('overview.openConfig') }}</span>
        </FhButton>
      </div>
      <div class="overview-metrics">
        <div v-for="m in metrics" :key="m.key" class="metric">
          <span class="metric__k">{{ t(m.key) }}</span>
          <span class="metric__v" :class="{ mono: m.mono }">{{ m.value }}</span>
        </div>
      </div>
    </div>

    <!-- 常用操作 -->
    <div class="card quick-card">
      <div class="card-head">
        <h3>{{ t('overview.quickTitle') }}</h3>
        <span class="card-note">{{ t('overview.quickNote') }}</span>
      </div>
      <div class="quick-grid">
        <button v-for="q in quick" :key="q.key" class="quick-item" @click="app.activate(q.pane)">
          <component :is="q.icon" :size="18" />
          <span>{{ t(q.key) }}</span>
        </button>
        <button class="quick-item" @click="diagnoseOpen = true">
          <Stethoscope :size="18" />
          <span>{{ t('overview.diagnose') }}</span>
        </button>
      </div>
    </div>

    <DiagnoseDialog v-model:open="diagnoseOpen" />

    <!-- 本机动态：状态轮询合成的本地时间线，最近 5 条 -->
    <div class="card events-card">
      <div class="card-head">
        <h3>{{ t('overview.eventsTitle') }}</h3>
        <button class="tab" @click="app.activate('operations')">{{ t('overview.eventsOps') }}</button>
      </div>
      <ul class="event-list">
        <li v-if="!events.length" class="event-list__empty">{{ t('events.empty') }}</li>
        <li v-for="(e, i) in events" :key="i" :class="`event-list__item event-list__item--${e.kind}`">
          <span class="ev-dot" />
          <div>
            <b>{{ e.text }}</b>
            <small>{{ e.time }} · {{ t('events.systemMsg') }}</small>
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>

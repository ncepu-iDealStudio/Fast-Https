<script setup lang="ts">
/**
 * 监控中心：合并原「概览 / 状态矩阵 / 运行快照」三页。
 * 三页原本重复拉取 GetOverview、runtime status、monitor status，
 * 合并后一次 load() 拉齐所有数据，各区块只显示一次，不再重复。
 *
 * 布局借鉴原型：顶部服务状态卡 → 运行指标 → 组件健康 → 明细 → 最近动态。
 */
import { computed, onMounted, ref } from 'vue';
import {
  Play, Square, RotateCw, FolderOpen, Stethoscope, RefreshCw,
  Activity, Globe, ShieldCheck, Server, Zap, AlertCircle, Clock,
} from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useOverviewStore } from '../../stores/overview';
import { runCommand } from '../../services/api/command';
import {
  startServer, stopServer, reloadServer, openConfigDir,
} from '../../services/api/app';

const { t } = useI18n();
const ov = useOverviewStore();

interface RuntimeData {
  state?: string; pid?: number; uptime_seconds?: number;
  connections?: Record<string, number>;
  requests?: Record<string, number>;
  responses?: Record<string, number>;
  upstream?: Record<string, number>;
}
interface MonitorStatus {
  status?: string; healthy?: boolean; version?: string;
  last_collect_at?: string; last_error?: string;
}
interface MonitorEvent { kind?: string; at?: string; time?: string; message?: string }

const rt = ref<RuntimeData | null>(null);
const monitor = ref<MonitorStatus | null>(null);
const events = ref<MonitorEvent[]>([]);
const loading = ref(false);
const notice = ref('');

const running = computed(() => ov.data?.running ?? false);

function durationText(seconds: number): string {
  const s = Number(seconds) || 0;
  if (s <= 0) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? t('runtime.durationHM', { h, m }) : t('runtime.durationM', { m });
}

function fmtTime(s?: string): string {
  return (s ?? '').replace('T', ' ').slice(0, 19) || '—';
}

/** 一次性拉齐：overview + runtime status + runtime stats + monitor status + events。
 *  服务未启动时 runtime / monitor 命令失败属正常，静默留空，不显示报错。 */
async function load(): Promise<void> {
  loading.value = true;
  try {
    await ov.refresh(true);
  } catch { /* 服务未启动不算错误 */ }
  try {
    const r = await runCommand(['runtime', 'status']);
    const s = await runCommand(['runtime', 'stats']);
    const sd = (s.data ?? {}) as { responses?: Record<string, number>; upstream?: Record<string, number> };
    rt.value = {
      ...(r.data ?? {}),
      responses: sd.responses ?? {},
      upstream: sd.upstream ?? {},
    } as RuntimeData;
  } catch { rt.value = null; }
  try {
    const st = await runCommand(['monitor', 'status']);
    monitor.value = (st.data ?? null) as MonitorStatus | null;
  } catch { monitor.value = null; }
  try {
    const ev = await runCommand(['monitor', 'events', '--limit', '8']);
    const d = (ev.data ?? {}) as { events?: MonitorEvent[] };
    events.value = d.events ?? [];
  } catch { events.value = []; }
  loading.value = false;
}

async function toggleServer(): Promise<void> {
  try {
    if (running.value) await stopServer(); else await startServer();
    await load();
  } catch { await load(); }
}
async function reload(): Promise<void> {
  try { await reloadServer(); } finally { await load(); }
}
async function openDir(): Promise<void> {
  const p = ov.data?.paths?.config;
  if (p) { try { await openConfigDir(p); } catch { /* ignore */ } }
}

const portsText = computed(() => (ov.data?.ports ?? []).join(', ') || '—');
const conns = computed(() => rt.value?.connections ?? {});
const reqs = computed(() => rt.value?.requests ?? {});
const resp = computed(() => rt.value?.responses ?? {});
const upstream = computed(() => rt.value?.upstream ?? {});

const metrics = computed(() => [
  { key: 'runtime.cardConn', value: conns.value.active ?? 0, sub: t('monitor.healthy'), ico: Activity, tone: '' },
  { key: 'runtime.cardReq', value: reqs.value.total ?? 0, sub: 'requests', ico: Zap, tone: '' },
  { key: 'runtime.cardResp', value: (resp.value['4xx'] ?? 0) + (resp.value['5xx'] ?? 0), sub: 'errors', ico: AlertCircle, tone: 'warn' },
  { key: 'runtime.cardUpstream', value: Object.values(upstream.value).reduce((a, b) => a + Number(b), 0), sub: 'upstream', ico: Server, tone: '' },
]);

const health = computed(() => [
  { title: 'HTTP', ico: Globe, status: running.value ? t('status.running') : t('status.stopped'), ok: running.value },
  { title: 'Monitor', ico: Activity, status: monitor.value?.healthy ? t('monitor.statusRunning') : t('monitor.notInstalled'), ok: !!monitor.value?.healthy },
  { title: t('nav.sites'), ico: Server, status: `${ov.data?.siteCount ?? 0}`, ok: true },
  { title: t('nav.certs'), ico: ShieldCheck, status: `${ov.data?.tlsCount ?? 0}`, ok: true },
]);

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body monitor-page">
    <!-- 顶部：服务状态卡 -->
    <section class="hero" :class="{ 'hero--off': !running }">
      <div class="hero__head">
        <div>
          <div class="hero__eyebrow">SERVICE STATUS</div>
          <h2 class="hero__title">
            {{ running ? t('status.running') : t('status.stopped') }}
            <span v-if="running" class="hero__live">LIVE</span>
          </h2>
          <p class="hero__sub">{{ running ? t('status.subRunning') : t('status.subStopped') }}</p>
        </div>
        <div class="hero__actions">
          <button class="hero__btn" :title="running ? t('overview.stop') : t('overview.start')" @click="toggleServer">
            <component :is="running ? Square : Play" :size="14" />
          </button>
          <button class="hero__btn" :title="t('overview.reload')" @click="reload"><RotateCw :size="14" /></button>
          <button class="hero__btn" :title="t('overview.diagnose')"><Stethoscope :size="14" /></button>
          <button class="hero__btn" :title="t('overview.openConfig')" @click="openDir"><FolderOpen :size="14" /></button>
          <button class="hero__btn" :title="t('common.refresh')" :disabled="loading" @click="load()"><RefreshCw :size="14" /></button>
        </div>
      </div>
      <div class="hero__facts">
        <div class="hero__fact"><small>PROCESS ID</small><b class="mono">{{ rt?.pid ?? ov.data?.pid ?? '—' }}</b></div>
        <div class="hero__fact"><small>UPTIME</small><b class="mono">{{ durationText(rt?.uptime_seconds ?? ov.data?.uptimeSeconds ?? 0) }}</b></div>
        <div class="hero__fact"><small>LISTENING</small><b class="mono">{{ portsText }}</b></div>
        <div class="hero__fact"><small>INSTANCE</small><b>{{ t('nav.account') }}</b></div>
        <div class="hero__fact"><small>MONITOR</small><b :class="{ 'is-ok': monitor?.healthy }">{{ monitor?.healthy ? 'Healthy' : '—' }}</b></div>
      </div>
    </section>

    <p v-if="notice" class="notice">{{ notice }}</p>

    <!-- 运行指标 -->
    <div class="section-head"><span>{{ t('runtime.pageTitle') }}</span><small>{{ t('runtime.pageNote') }}</small></div>
    <div class="metric-grid">
      <div v-for="m in metrics" :key="m.key" class="metric-card" :class="{ 'metric-card--warn': m.tone === 'warn' }">
        <div class="metric-card__top">
          <span>{{ t(m.key) }}</span>
          <component :is="m.ico" :size="16" />
        </div>
        <div class="metric-card__value mono">{{ m.value }}</div>
        <div class="metric-card__sub">{{ m.sub }}</div>
      </div>
    </div>

    <!-- 组件健康 + 最近动态 -->
    <div class="two-col">
      <section class="panel">
        <div class="panel__head"><h3>{{ t('nav.matrix') }}</h3></div>
        <div class="health-grid">
          <div v-for="h in health" :key="h.title" class="health-item">
            <div class="health-item__ico"><component :is="h.ico" :size="16" /></div>
            <div class="health-item__body">
              <b>{{ h.title }}</b>
              <small>{{ h.status }}</small>
            </div>
            <span class="dot" :class="h.ok ? 'state-ok' : 'state-off'" />
          </div>
        </div>
      </section>

      <section class="panel">
        <div class="panel__head">
          <h3>{{ t('overview.eventsTitle') }}</h3>
          <Clock :size="14" />
        </div>
        <ul class="event-list">
          <li v-if="!events.length" class="event-list__empty">{{ t('events.empty') }}</li>
          <li v-for="(e, i) in events" :key="i" class="event-list__item">
            <span class="ev-dot" />
            <div><b>{{ e.kind || e.message || '—' }}</b><small>{{ fmtTime(e.at || e.time) }}</small></div>
          </li>
        </ul>
      </section>
    </div>

    <!-- 运行数据明细 -->
    <section class="panel">
      <div class="panel__head"><h3>{{ t('matrix.runtimeTitle') }}</h3></div>
      <div class="detail-grid">
        <div class="detail-group">
          <div class="detail-group__title">{{ t('runtime.cardConn') }}</div>
          <div v-for="(v, k) in conns" :key="k" class="detail-row">
            <span class="mono">{{ k }}</span><b class="mono">{{ v }}</b>
          </div>
        </div>
        <div class="detail-group">
          <div class="detail-group__title">{{ t('runtime.cardReq') }}</div>
          <div v-for="(v, k) in reqs" :key="k" class="detail-row">
            <span class="mono">{{ k }}</span><b class="mono">{{ v }}</b>
          </div>
        </div>
        <div class="detail-group">
          <div class="detail-group__title">{{ t('runtime.cardResp') }}</div>
          <div v-for="(v, k) in resp" :key="k" class="detail-row">
            <span class="mono">{{ k }}</span><b class="mono">{{ v }}</b>
          </div>
        </div>
        <div class="detail-group">
          <div class="detail-group__title">{{ t('runtime.cardUpstream') }}</div>
          <div v-for="(v, k) in upstream" :key="k" class="detail-row">
            <span class="mono">{{ k }}</span><b class="mono">{{ v }}</b>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

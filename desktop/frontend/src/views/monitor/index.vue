<script setup lang="ts">
/**
 * 监控中心（编排壳）：只负责拉数据 + 组装子组件。
 * 概览 / 状态矩阵 / 运行快照 已合并于此，数据只拉一次、区块不重复。
 */
import { computed, onMounted } from 'vue';
import { Activity, Globe, ShieldCheck, Server, Zap, AlertCircle } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { startServer, stopServer, reloadServer, openConfigDir } from '../../services/api/app';
import { useMonitorData } from './composables/useMonitorData';
import HeroCard from './components/HeroCard.vue';
import MetricCards from './components/MetricCards.vue';
import HealthPanel from './components/HealthPanel.vue';
import DetailPanel from './components/DetailPanel.vue';
import EventPanel from './components/EventPanel.vue';

const { t } = useI18n();
const { ov, rt, monitor, events, loading, load } = useMonitorData();

const running = computed(() => ov.data?.running ?? false);

function durationText(seconds: number): string {
  const s = Number(seconds) || 0;
  if (s <= 0) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? t('runtime.durationHM', { h, m }) : t('runtime.durationM', { m });
}

const portsText = computed(() => (ov.data?.ports ?? []).join(', ') || '—');
const conns = computed(() => rt.value?.connections ?? {});
const reqs = computed(() => rt.value?.requests ?? {});
const resp = computed(() => rt.value?.responses ?? {});
const upstream = computed(() => rt.value?.upstream ?? {});

const metrics = computed(() => [
  { key: t('runtime.cardConn'), value: conns.value.active ?? 0, sub: 'active', ico: Activity, tone: '' as const, trend: [4, 6, 5, 9, 7, 12, 10] },
  { key: t('runtime.cardReq'), value: reqs.value.total ?? 0, sub: 'requests', ico: Zap, tone: '' as const, trend: [2, 4, 6, 5, 8, 11, 13] },
  { key: t('runtime.cardResp'), value: (resp.value['4xx'] ?? 0) + (resp.value['5xx'] ?? 0), sub: 'errors', ico: AlertCircle, tone: 'warn' as const, trend: [1, 1, 2, 1, 3, 2, 2] },
  { key: t('runtime.cardUpstream'), value: Object.values(upstream.value).reduce((a, b) => a + Number(b), 0), sub: 'upstream', ico: Server, tone: '' as const, trend: [3, 3, 4, 4, 5, 5, 6] },
]);

const health = computed(() => [
  { title: 'HTTP', ico: Globe, status: running.value ? t('status.running') : t('status.stopped'), ok: running.value },
  { title: 'Monitor', ico: Activity, status: monitor.value?.healthy ? t('monitor.statusRunning') : t('monitor.notInstalled'), ok: !!monitor.value?.healthy },
  { title: t('nav.sites'), ico: Server, status: `${ov.data?.siteCount ?? 0}`, ok: true },
  { title: t('nav.certs'), ico: ShieldCheck, status: `${ov.data?.tlsCount ?? 0}`, ok: true },
]);

function rowsOf(rec: Record<string, number>): { k: string; v: number }[] {
  return Object.entries(rec).map(([k, v]) => ({ k, v }));
}
const detailGroups = computed(() => [
  { title: t('runtime.cardConn'), rows: rowsOf(conns.value) },
  { title: t('runtime.cardReq'), rows: rowsOf(reqs.value) },
  { title: t('runtime.cardResp'), rows: rowsOf(resp.value) },
  { title: t('runtime.cardUpstream'), rows: rowsOf(upstream.value) },
]);

async function toggleServer(): Promise<void> {
  try { if (running.value) await stopServer(); else await startServer(); } finally { await load(); }
}
async function reload(): Promise<void> {
  try { await reloadServer(); } finally { await load(); }
}
async function openDir(): Promise<void> {
  const p = ov.data?.paths?.config;
  if (p) { try { await openConfigDir(p); } catch { /* ignore */ } }
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="main__body monitor-page">
    <HeroCard
      :running="running"
      :pid="rt?.pid ?? ov.data?.pid"
      :uptime="durationText(rt?.uptime_seconds ?? ov.data?.uptimeSeconds ?? 0)"
      :ports="portsText"
      :monitor-healthy="!!monitor?.healthy"
      :loading="loading"
      @toggle="toggleServer" @reload="reload" @refresh="load"
      @open-dir="openDir"
    />

    <div class="section-head"><span>{{ t('runtime.pageTitle') }}</span><small>{{ t('runtime.pageNote') }}</small></div>
    <MetricCards :metrics="metrics" />

    <div class="two-col">
      <HealthPanel :items="health" :title="t('nav.matrix')" />
      <EventPanel :events="events" :title="t('overview.eventsTitle')" />
    </div>

    <DetailPanel :groups="detailGroups" :title="t('matrix.runtimeTitle')" />
  </div>
</template>

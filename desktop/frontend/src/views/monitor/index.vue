<script setup lang="ts">
/**
 * 监控中心（编排壳）：页头 + 服务状态卡 + 运行概览 + 组件健康 + 明细/动态。
 * 概览 / 状态矩阵 / 运行快照 三页合并于此，数据只拉一次、区块不重复。
 */
import { computed, onMounted, ref } from 'vue';
import { Activity, Globe, ShieldCheck, Server, Zap, AlertCircle, FolderOpen } from '@lucide/vue';
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
const busy = ref(false);
const notice = ref('');

const running = computed(() => ov.data?.running ?? false);

function durationText(seconds: number): string {
  const s = Number(seconds) || 0;
  if (s <= 0) return '\u2014';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? t('runtime.durationHM', { h, m }) : t('runtime.durationM', { m });
}

const portsText = computed(() => (ov.data?.ports ?? []).join(', ') || '\u2014');
const conns = computed(() => rt.value?.connections ?? {});
const reqs = computed(() => rt.value?.requests ?? {});
const resp = computed(() => rt.value?.responses ?? {});
const upstream = computed(() => rt.value?.upstream ?? {});

function spark(seed: number): number[] {
  return Array.from({ length: 7 }, (_, i) => 6 + Math.sin(seed + i) * 4 + i * 1.2);
}

const metrics = computed(() => [
  { label: t('runtime.cardConn'), value: conns.value.active ?? 0, sub: t('status.subRunning'), subTone: 'ok' as const, ico: Activity, icoTone: 'blue' as const, trend: spark(0) },
  { label: t('runtime.cardReq'), value: reqs.value.total ?? 0, sub: t('runtime.cardReq'), subTone: 'ok' as const, ico: Zap, icoTone: 'green' as const, trend: spark(2) },
  { label: t('runtime.cardResp'), value: resp.value.total ?? Object.values(resp.value).reduce((a, b) => a + Number(b), 0), sub: 'requests', subTone: 'muted' as const, ico: Server, icoTone: 'orange' as const, trend: spark(4) },
  { label: t('runtime.cardUpstream'), value: Object.values(upstream.value).reduce((a, b) => a + Number(b), 0), sub: 'errors - rejected', subTone: 'ok' as const, ico: AlertCircle, icoTone: 'purple' as const, trend: spark(6) },
]);

const health = computed(() => [
  {
    title: 'HTTP', ico: Activity, ok: running.value, status: running.value ? t('status.running') : t('status.stopped'),
    desc: `${t('overview.metricPorts')} :80 / :443`, action: t('matrix.title'),
  },
  {
    title: 'Monitor', ico: Activity, ok: !!monitor.value?.healthy,
    status: monitor.value?.healthy ? t('monitor.statusRunning') : t('monitor.notInstalled'),
    desc: `${t('monitor.lastCollect')} ${monitor.value?.last_collect_at?.slice(0, 19).replace('T', ' ') ?? '\u2014'}`,
    action: t('overview.eventsTitle'),
  },
  {
    title: t('nav.sites'), ico: Globe, ok: true,
    status: t('settings.current'),
    desc: `${ov.data?.siteCount ?? 0} ${t('nav.sites')}`, action: t('nav.sites'),
  },
  {
    title: t('nav.certs'), ico: ShieldCheck, ok: true,
    status: t('settings.current'),
    desc: `${ov.data?.tlsCount ?? 0} ${t('nav.certs')}`, action: t('nav.certs'),
  },
]);

async function toggleServer(): Promise<void> {
  busy.value = true;
  try { if (running.value) await stopServer(); else await startServer(); } finally { busy.value = false; await load(); }
}
async function reload(): Promise<void> {
  busy.value = true;
  try { await reloadServer(); } finally { busy.value = false; await load(); }
}
async function openDir(): Promise<void> {
  const p = ov.data?.paths?.config;
  if (p) { try { await openConfigDir(p); } catch { /* ignore */ } }
}

onMounted(() => { void load(); });
</script>

<template>
  <div class="monitor-page">
    <!-- 妞ら潧銇?-->
    <div class="page-head">
      <div class="page-head__text">
        <div class="page-head__eyebrow">SERVICE CONTROL CENTER</div>
        <h1>{{ t('monitor.center') }}</h1>
        <p>{{ t('monitor.centerNote') }}</p>
      </div>
      <div class="page-head__actions">
        <button class="icon-btn page-head__refresh" :title="t('overview.openConfig')" @click="openDir">
          <FolderOpen :size="14" />
        </button>
      </div>
    </div>

    <!-- 运行概览 -->
    <HeroCard
      :running="running"
      :pid="rt?.pid ?? ov.data?.pid"
      :uptime="durationText(rt?.uptime_seconds ?? ov.data?.uptimeSeconds ?? 0)"
      :ports="portsText"
      :monitor-healthy="!!monitor?.healthy"
      :loading="loading"
      @toggle="toggleServer" @reload="reload" @refresh="load"
      @open-dir="openDir" @diagnose="notice = ''"
    />

    <p v-if="notice" class="notice">{{ notice }}</p>

    <!-- 鏉╂劘顢戝鍌濐潔 -->
    <div class="section-head"><span>{{ t('runtime.pageTitle') }}</span><small>{{ t('runtime.pageNote') }}</small></div>
    <MetricCards :metrics="metrics" />

    <!-- 缂佸嫪娆㈤崑銉ユ倣 -->
    <HealthPanel :items="health" :title="t('nav.matrix')" :note="t('runtime.pageNote')" @detail="notice = $event" />

    <!-- 明细 + 最近动态 -->
    <div class="two-col">
      <DetailPanel
        :conns="conns" :reqs="reqs" :resp="resp" :upstream="upstream"
        :title="t('matrix.runtimeTitle')" :note="t('runtime.pageNote')"
      />
      <EventPanel :events="events" :title="t('overview.eventsTitle')" :link="t('overview.eventsOps')" @open="notice = ''" />
    </div>
  </div>
</template>

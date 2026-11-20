<script setup lang="ts">
/**
 * 监控中心（编排壳）：hero + 运行概览 + 组件健康 + 明细/事件双栏。
 * 概览 / 状态矩阵 / 运行快照 三页合并于此，一次拉齐所有数据。
 * 页面内不重复大标题——标题由顶栏与侧栏承担。
 */
import { computed, ref } from 'vue';
import { Activity, Globe, Database, ShieldCheck, ScrollText, Zap, AlertCircle, ArrowLeftRight, Check } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useAppStore, type PaneKey } from '../../stores/app';
import { startServer, stopServer, reloadServer } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import { usePolling } from '../../composables/usePolling';
import { useMonitorData } from './composables/useMonitorData';
import HeroCard from './components/HeroCard.vue';
import MetricCards from './components/MetricCards.vue';
import type { MetricCard } from './components/MetricCards.vue';
import HealthPanel from './components/HealthPanel.vue';
import type { HealthCard } from './components/HealthPanel.vue';
import DetailPanel from './components/DetailPanel.vue';
import EventPanel from './components/EventPanel.vue';
import DiagnoseDialog from '../../components/common/DiagnoseDialog.vue';

const { t } = useI18n();
const app = useAppStore();
const { ov, rt, monitor, events, load } = useMonitorData();

const busy = ref(false);
const notice = ref('');
/** 健康检查弹窗开关 */
const diagOpen = ref(false);
const EVENT_LIMIT = 8;

const running = computed(() => ov.data?.running ?? false);

function durationText(seconds: number): string {
  const s = Number(seconds) || 0;
  if (s <= 0) return '';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}h ${m}m ${String(sec).padStart(2, '0')}s`;
  if (m > 0) return `${m}m ${String(sec).padStart(2, '0')}s`;
  return `${sec}s`;
}

const portsText = computed(() => (ov.data?.ports ?? []).join('  ') || '');
const conns = computed(() => rt.value?.connections ?? {});
const reqs = computed(() => rt.value?.requests ?? {});
const resp = computed(() => rt.value?.responses ?? {});
const upstream = computed(() => rt.value?.upstream ?? {});

/** 四张指标卡：数值全部真实，趋势线只在有历史采样时出现。 */
const cards = computed<MetricCard[]>(() => {
  const errTotal = (resp.value['4xx'] ?? 0) + (resp.value['5xx'] ?? 0) + (conns.value.rejected_total ?? 0);
  return [
    {
      label: t('runtime.cardConn'), value: conns.value.active ?? 0,
      sub: running.value ? t('monitor.normal') : t('status.stopped'),
      subTone: running.value ? 'ok' : 'muted', ico: ArrowLeftRight, tone: 'blue',
      trend: rt.value?.history?.connActive,
    },
    {
      label: t('monitor.metricAccepted'), value: conns.value.accepted_total ?? 0,
      sub: t('monitor.accepted'), subTone: 'ok', ico: Check, tone: 'green',
      trend: rt.value?.history?.connAccepted,
    },
    {
      label: t('monitor.metricRequests'), value: reqs.value.total ?? 0,
      sub: t('monitor.requests'), subTone: 'ok', ico: Zap, tone: 'orange',
      trend: rt.value?.history?.reqTotal,
    },
    {
      label: t('monitor.metricErrors'), value: errTotal,
      sub: errTotal === 0 ? t('monitor.allNormal') : t('monitor.hasErrors'),
      subTone: errTotal === 0 ? 'ok' : 'warn', ico: AlertCircle, tone: 'purple',
      trend: rt.value?.history?.errors, warn: errTotal > 0,
    },
  ];
});

/** 组件健康四卡：站点/证书数来自 overview，Monitor 来自 monitor status。 */
const health = computed<HealthCard[]>(() => {
  const days = ov.data?.nearestCertDays;
  return [
    {
      title: t('nav.logs'), ico: ScrollText, ok: running.value,
      status: running.value ? t('monitor.normal') : t('status.stopped'),
      desc: portsText.value ? `${t('overview.metricPorts')} ${portsText.value}` : t('status.subStopped'),
      meta: running.value && ov.data?.pid ? `PID ${ov.data.pid}` : '',
      action: t('logs.openLog'), pane: 'logs',
    },
    {
      title: 'Monitor', ico: Activity, ok: !!monitor.value?.healthy,
      status: monitor.value?.healthy ? t('monitor.statusRunning') : t('monitor.notInstalled'),
      desc: monitor.value?.healthy ? t('monitor.collectEnabled') : t('monitor.notInstalled'),
      meta: monitor.value?.last_collect_at
        ? `${t('monitor.lastCollect')} ${monitor.value.last_collect_at.replace('T', ' ').slice(11, 19)}`
        : '',
      action: t('monitor.viewEvents'), pane: 'operations',
    },
    {
      title: t('nav.sites'), ico: Globe, ok: true, status: t('settings.current'),
      desc: t('monitor.sitesManaged', { n: ov.data?.siteCount ?? 0 }),
      meta: '',
      action: t('monitor.manageSites'), pane: 'sites',
    },
    {
      title: t('nav.cache'), ico: Database, ok: true, status: t('settings.current'),
      desc: t('monitor.cacheZones', { n: ov.data?.cacheZoneCount ?? 0 }),
      meta: '',
      action: t('monitor.manageCache'), pane: 'cache',
    },
    {
      title: t('nav.certs'), ico: ShieldCheck, ok: true, status: t('settings.current'),
      desc: t('monitor.certsConfigured', { n: ov.data?.tlsCount ?? 0 }),
      meta: days && days > 0 ? t('monitor.expireIn', { n: days }) : '',
      action: t('monitor.manageCerts'), pane: 'certs',
    },
  ];
});

async function toggleServer(): Promise<void> {
  busy.value = true;
  notice.value = '';
  try {
    if (running.value) await stopServer(); else await startServer();
  } catch (e) {
    notice.value = messageOf(e);
  } finally {
    busy.value = false;
    await load();
  }
}

async function reload(): Promise<void> {
  busy.value = true;
  notice.value = '';
  try {
    await reloadServer();
  } catch (e) {
    notice.value = messageOf(e);
  } finally {
    busy.value = false;
    await load();
  }
}

function go(pane: string): void {
  app.activate(pane as PaneKey);
}

// 监控中心常驻页面：每 5 秒刷新一次运行快照、健康状态与事件。
usePolling(() => { void load(); }, 5000, computed(() => true));
</script>

<template>
  <div class="monitor-page">
    <HeroCard
      :running="running"
      :pid="rt?.pid ?? ov.data?.pid"
      :uptime="durationText(rt?.uptime_seconds ?? ov.data?.uptimeSeconds ?? 0)"
      :ports="portsText"
      :monitor-healthy="!!monitor?.healthy"
      :busy="busy"
      @toggle="toggleServer" @reload="reload" @diagnose="diagOpen = true"
    />

    <p v-if="notice" class="notice notice--err">{{ notice }}</p>

    <div class="sec-head">
      <h2>{{ t('monitor.sectionOverview') }}</h2>
      <span class="sec-head__note">{{ t('monitor.sectionOverviewNote') }}</span>
    </div>
    <MetricCards :cards="cards" />

    <div class="sec-head">
      <h2>{{ t('monitor.sectionHealth') }}</h2>
      <span class="sec-head__note">{{ t('monitor.sectionHealthNote') }}</span>
    </div>
    <HealthPanel :cards="health" @go="go" />

    <div class="monitor-bottom">
      <DetailPanel
        :conns="conns" :reqs="reqs" :resp="resp" :upstream="upstream"
        :title="t('monitor.sectionMetrics')" :note="t('monitor.processMetrics')"
      />
      <EventPanel
        :events="events.slice(0, EVENT_LIMIT)" :title="t('overview.eventsTitle')" :limit="EVENT_LIMIT"
      />
    </div>

    <DiagnoseDialog v-model:open="diagOpen" />
  </div>
</template>
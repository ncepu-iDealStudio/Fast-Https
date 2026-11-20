import { ref } from 'vue';
import { runCommand } from '../../../services/api/command';
import { useOverviewStore } from '../../../stores/overview';

/** 趋势线保留的采样点数。每 2 秒一次 → 约 1 分钟的历史。 */
const HISTORY_LEN = 30;

export interface RuntimeHistory {
  connActive: number[];
  connAccepted: number[];
  reqTotal: number[];
  errors: number[];
}

export interface RuntimeData {
  state?: string;
  pid?: number;
  uptime_seconds?: number;
  connections?: Record<string, number>;
  requests?: Record<string, number>;
  responses?: Record<string, number>;
  upstream?: Record<string, number>;
  /** 真实采样历史：每次 load 追加一个点，不编造 */
  history?: RuntimeHistory;
}

export interface MonitorStatus {
  status?: string;
  healthy?: boolean;
  version?: string;
  last_collect_at?: string;
  last_error?: string;
}

/** 与后端 monitor.Event 对齐：type / rule / severity / message / snapshot。 */
export interface MonitorEvent {
  time?: string;
  type?: string;
  rule?: string;
  severity?: string;
  message?: string;
  snapshot?: {
    state?: string;
    pid?: number;
    requests_total?: number;
    responses_5xx_total?: number;
    requests_delta?: number;
    responses_5xx_delta?: number;
  };
}

/**
 * 监控中心数据：一次拉齐 overview + runtime status/stats + monitor status/events。
 * 服务未启动时 runtime / monitor 命令失败属正常，静默留空不报错。
 * 趋势线数据来自本会话内累积的真实采样，不做插值或模拟。
 */
export function useMonitorData() {
  const ov = useOverviewStore();
  const rt = ref<RuntimeData | null>(null);
  const monitor = ref<MonitorStatus | null>(null);
  const events = ref<MonitorEvent[]>([]);
  const loading = ref(false);

  const hist: RuntimeHistory = { connActive: [], connAccepted: [], reqTotal: [], errors: [] };

  function pushSample(conns: Record<string, number>, reqs: Record<string, number>, resp: Record<string, number>): void {
    const push = (arr: number[], v: number): void => {
      arr.push(v);
      if (arr.length > HISTORY_LEN) arr.shift();
    };
    push(hist.connActive, conns.active ?? 0);
    push(hist.connAccepted, conns.accepted_total ?? 0);
    push(hist.reqTotal, reqs.total ?? 0);
    push(hist.errors, (resp['4xx'] ?? 0) + (resp['5xx'] ?? 0) + (conns.rejected_total ?? 0));
  }

  function clearHistory(): void {
    hist.connActive.length = 0;
    hist.connAccepted.length = 0;
    hist.reqTotal.length = 0;
    hist.errors.length = 0;
  }

  async function load(): Promise<void> {
    // 轮询可能在上一次还没跑完时触发：直接跳过，避免请求堆积。
    if (loading.value) return;
    loading.value = true;

    try {
      await ov.refresh(true);
    } catch { /* 服务未启动不算错误 */ }

    try {
      const [r, s] = await Promise.all([
        runCommand(['runtime', 'status']),
        runCommand(['runtime', 'stats']),
      ]);
      const sd = (s.data ?? {}) as { responses?: Record<string, number>; upstream?: Record<string, number> };
      const base = (r.data ?? {}) as RuntimeData;
      const conns = base.connections ?? {};
      const reqs = base.requests ?? {};
      const resp = sd.responses ?? {};
      pushSample(conns, reqs, resp);
      rt.value = {
        ...base,
        responses: resp,
        upstream: sd.upstream ?? {},
        history: {
          connActive: [...hist.connActive],
          connAccepted: [...hist.connAccepted],
          reqTotal: [...hist.reqTotal],
          errors: [...hist.errors],
        },
      };
    } catch {
      rt.value = null;
      // 服务停掉后清空历史，避免恢复时把停机期间的断点连成线
      clearHistory();
    }

    const [st, ev] = await Promise.all([
      runCommand(['monitor', 'status']).catch(() => null),
      runCommand(['monitor', 'events', '--limit', '8']).catch(() => null),
    ]);
    monitor.value = st ? ((st.data ?? null) as MonitorStatus | null) : null;
    events.value = ev ? (((ev.data ?? {}) as { events?: MonitorEvent[] }).events ?? []) : [];

    loading.value = false;
  }

  return { ov, rt, monitor, events, loading, load };
}

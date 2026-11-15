import { ref } from 'vue';
import { runCommand } from '../../../services/api/command';
import { useOverviewStore } from '../../../stores/overview';

export interface RuntimeData {
  state?: string; pid?: number; uptime_seconds?: number;
  connections?: Record<string, number>;
  requests?: Record<string, number>;
  responses?: Record<string, number>;
  upstream?: Record<string, number>;
}
export interface MonitorStatus {
  status?: string; healthy?: boolean; version?: string;
  last_collect_at?: string; last_error?: string;
}
export interface MonitorEvent { kind?: string; at?: string; time?: string; message?: string }

/** 监控中心数据：一次拉齐 overview + runtime status/stats + monitor status/events。
 *  服务未启动时 runtime / monitor 命令失败属正常，静默留空不报错。 */
export function useMonitorData() {
  const ov = useOverviewStore();
  const rt = ref<RuntimeData | null>(null);
  const monitor = ref<MonitorStatus | null>(null);
  const events = ref<MonitorEvent[]>([]);
  const loading = ref(false);

  async function load(): Promise<void> {
    loading.value = true;
    try { await ov.refresh(true); } catch { /* 服务未启动不算错误 */ }
    try {
      const r = await runCommand(['runtime', 'status']);
      const s = await runCommand(['runtime', 'stats']);
      const sd = (s.data ?? {}) as { responses?: Record<string, number>; upstream?: Record<string, number> };
      rt.value = { ...(r.data ?? {}), responses: sd.responses ?? {}, upstream: sd.upstream ?? {} } as RuntimeData;
    } catch { rt.value = null; }
    try {
      const st = await runCommand(['monitor', 'status']);
      monitor.value = (st.data ?? null) as MonitorStatus | null;
    } catch { monitor.value = null; }
    try {
      const ev = await runCommand(['monitor', 'events', '--limit', '8']);
      events.value = ((ev.data ?? {}) as { events?: MonitorEvent[] }).events ?? [];
    } catch { events.value = []; }
    loading.value = false;
  }

  return { ov, rt, monitor, events, loading, load };
}

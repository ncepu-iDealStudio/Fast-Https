import { ref } from 'vue';
import { runCommand } from '../../../services/api/command';
import { messageOf } from '../../../services/api/base';

/** 与 operation list 的 JSON 对齐；版本页与审计页共用。 */
export interface Operation {
  operation_id?: string; command?: string; status?: string;
  created_at?: string; started_at?: string; finished_at?: string;
  phase?: string; owner_pid?: number; version?: string;
  result?: Record<string, unknown>;
  error?: { code?: string; message?: string; details?: { code?: string; message?: string }[] } | null;
}

export interface OpChange {
  path?: string; op?: string; value?: unknown; old?: unknown; new?: unknown; changed?: boolean;
}

const MAX_LIMIT = 1000;

/**
 * 操作记录列表：分页、失败态与刷新逻辑。
 * extraArgs 用于审计页的 --status 过滤；版本页传空。
 */
export function useOperations(extraArgs: () => string[] = () => []) {
  const rows = ref<Operation[]>([]);
  const loading = ref(false);
  const error = ref('');
  const limit = ref(50);
  const allLoaded = ref(false);

  async function load(reset = true): Promise<void> {
    if (loading.value) return;
    if (reset) { limit.value = 50; allLoaded.value = false; }
    loading.value = true;
    error.value = '';
    try {
      const r = await runCommand(['operation', 'list', '--limit', String(limit.value), ...extraArgs()]);
      const d = r.data as { operations?: Operation[] } | null;
      rows.value = d?.operations ?? [];
      if (rows.value.length < limit.value) allLoaded.value = true;
    } catch (e) {
      error.value = messageOf(e);
      if (reset) rows.value = [];
    } finally {
      loading.value = false;
    }
  }

  async function loadMore(): Promise<void> {
    if (loading.value || allLoaded.value || limit.value >= MAX_LIMIT) return;
    limit.value = Math.min(MAX_LIMIT, limit.value * 2);
    await load(false);
  }

  return { rows, loading, error, limit, allLoaded, load, loadMore };
}

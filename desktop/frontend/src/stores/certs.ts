import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { Cert, LogEntry } from '../types/model';
import { listCerts } from '../services/api/cert';
import { tailLog } from '../services/api/log';
import { messageOf } from '../services/api/base';

export const useCertsStore = defineStore('certs', () => {
  const items = ref<Cert[]>([]);
  const loading = ref(false);
  const error = ref('');

  async function refresh(): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      items.value = await listCerts();
    } catch (e) {
      error.value = messageOf(e);
    } finally {
      loading.value = false;
    }
  }

  return { items, loading, error, refresh };
});

export type LogKind = 'access' | 'error' | 'system' | 'safe';

export const useLogsStore = defineStore('logs', () => {
  const kind = ref<LogKind>('access');
  const entries = ref<LogEntry[]>([]);
  const loading = ref(false);
  const error = ref('');

  async function load(next?: LogKind): Promise<void> {
    if (next) kind.value = next;
    loading.value = true;
    error.value = '';
    try {
      entries.value = await tailLog(kind.value, 200);
    } catch (e) {
      error.value = messageOf(e);
    } finally {
      loading.value = false;
    }
  }

  return { kind, entries, loading, error, load };
});

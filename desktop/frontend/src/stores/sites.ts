import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { Site } from '../types/model';
import { listSites, setSiteEnabled, deleteSite } from '../services/api/site';
import { messageOf } from '../services/api/base';

export const useSitesStore = defineStore('sites', () => {
  const items = ref<Site[]>([]);
  const loading = ref(false);
  const error = ref('');

  async function refresh(): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      items.value = await listSites();
    } catch (e) {
      error.value = messageOf(e);
    } finally {
      loading.value = false;
    }
  }

  async function toggle(id: string, enabled: boolean): Promise<string> {
    const msg = await setSiteEnabled(id, enabled);
    await refresh();
    return msg;
  }

  async function remove(id: string): Promise<string> {
    const msg = await deleteSite(id);
    await refresh();
    return msg;
  }

  return { items, loading, error, refresh, toggle, remove };
});

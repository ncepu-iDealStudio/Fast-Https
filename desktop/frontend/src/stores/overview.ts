import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { Overview } from '../types/model';
import { getOverview } from '../services/api/overview';
import { messageOf } from '../services/api/base';

export const useOverviewStore = defineStore('overview', () => {
  const data = ref<Overview | null>(null);
  const loading = ref(false);
  const error = ref('');

  async function refresh(): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      data.value = await getOverview();
    } catch (e) {
      error.value = messageOf(e);
    } finally {
      loading.value = false;
    }
  }

  return { data, loading, error, refresh };
});

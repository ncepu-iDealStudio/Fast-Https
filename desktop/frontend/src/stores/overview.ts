import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { Overview } from '../types/model';
import { getOverview } from '../services/api/overview';
import { messageOf } from '../services/api/base';

export const useOverviewStore = defineStore('overview', () => {
  const data = ref<Overview | null>(null);
  const loading = ref(false);
  const error = ref('');

  /**
   * silent=true 用于 2 秒轮询：只更新数据，不碰 loading——
   * 否则按钮每 2 秒闪一次 spinner，重载这类手动按钮看起来像在自动执行。
   */
  async function refresh(silent = false): Promise<void> {
    if (!silent) loading.value = true;
    error.value = '';
    try {
      data.value = await getOverview();
    } catch (e) {
      error.value = messageOf(e);
    } finally {
      if (!silent) loading.value = false;
    }
  }

  return { data, loading, error, refresh };
});

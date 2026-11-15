import { watch, onUnmounted, type Ref } from 'vue';
import { useIntervalFn } from '@vueuse/core';

/**
 * 可复用轮询：组件卸载时自动清理（VueUse 保证），不残留裸 setInterval。
 * enabled 变化时自动暂停/恢复——用于「切到某页才拉数据」，与原版行为一致。
 */
export function usePolling(fn: () => void, ms: number, enabled: Ref<boolean>): void {
  const { pause, resume, isActive } = useIntervalFn(fn, ms, { immediate: false });

  watch(enabled, (on) => {
    if (on) {
      fn();
      if (!isActive.value) resume();
    } else if (isActive.value) {
      pause();
    }
  }, { immediate: true });

  onUnmounted(() => pause());
}

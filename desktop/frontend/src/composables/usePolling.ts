import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue';
import { useIntervalFn } from '@vueuse/core';

/**
 * 可复用轮询：组件卸载时自动清理（VueUse 保证），不残留裸 setInterval。
 * enabled 变化时自动暂停/恢复——用于「切到某页才拉数据」，与原版行为一致。
 * 窗口隐藏（最小化、切到后台）时同样暂停，避免无意义的定时任务。
 */
export function usePolling(fn: () => void, ms: number, enabled: Ref<boolean>): void {
  const { pause, resume, isActive } = useIntervalFn(fn, ms, { immediate: false });
  const visible = ref(true);
  const active = computed(() => enabled.value && visible.value);

  watch(active, (on) => {
    if (on) {
      fn();
      if (!isActive.value) resume();
    } else if (isActive.value) {
      pause();
    }
  }, { immediate: true });

  function onVisibility(): void {
    visible.value = !document.hidden;
  }
  onMounted(() => {
    onVisibility();
    document.addEventListener('visibilitychange', onVisibility);
  });
  onUnmounted(() => {
    document.removeEventListener('visibilitychange', onVisibility);
    pause();
  });
}

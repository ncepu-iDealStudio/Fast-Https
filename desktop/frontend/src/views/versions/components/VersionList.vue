<script setup lang="ts">
/** 配置版本列表：一行一个版本，对比 / 回滚操作。 */
import { GitCompareArrows, Undo2 } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

interface Version {
  version?: string; active?: boolean; created_at?: string;
  created_by?: string; summary?: string; parent?: string;
}
defineProps<{ versions: Version[]; loading: boolean }>();
const emit = defineEmits<{ diff: [v: string]; rollback: [v: string] }>();
const { t } = useI18n();

function metaText(v: Version): string {
  return [v.created_at, v.created_by, v.summary].filter(Boolean).join(' · ');
}
</script>

<template>
  <div class="verlist">
    <div v-if="!versions.length && !loading" class="table__empty">{{ t('settings.versionsEmpty') }}</div>
    <div
      v-for="v in versions" :key="String(v.version)"
      class="verlist__row" :class="{ 'verlist__row--active': v.active }"
    >
      <span class="verlist__ver mono">
        {{ v.version }}
        <span v-if="v.active" class="badge badge--ok">{{ t('settings.current') }}</span>
      </span>
      <span class="verlist__meta muted">{{ metaText(v) || '—' }}</span>
      <span class="verlist__ops">
        <button class="icon-btn" :title="t('settings.diff')" @click="emit('diff', String(v.version))">
          <GitCompareArrows :size="14" />
        </button>
        <button v-if="!v.active" class="icon-btn" :title="t('settings.rollback')" @click="emit('rollback', String(v.version))">
          <Undo2 :size="14" />
        </button>
      </span>
    </div>
  </div>
</template>

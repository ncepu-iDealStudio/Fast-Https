<script setup lang="ts">
/** diff 结果表：配置项 / 变更 / 旧值 / 新值，增删改三色。 */
import { X } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';

interface Change { path?: string; op?: string; value?: unknown; old?: unknown; new?: unknown }
defineProps<{ version: string; changes: Change[]; identical: boolean }>();
const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();

function showVal(v: unknown): string {
  if (v === undefined || v === null) return '—';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}
function opLabel(op?: string): string {
  if (op === 'added') return t('settings.diffOpAdded');
  if (op === 'removed') return t('settings.diffOpRemoved');
  return t('settings.diffOpModified');
}
function opClass(op?: string): string {
  if (op === 'added') return 'diffrow--add';
  if (op === 'removed') return 'diffrow--del';
  return 'diffrow--mod';
}
</script>

<template>
  <div class="card">
    <div class="card-head">
      <h3>{{ t('settings.diff') }} · <span class="mono">{{ version }}</span></h3>
      <button class="icon-btn" :title="t('common.close')" @click="emit('close')"><X :size="14" /></button>
    </div>
    <div v-if="identical" class="notice">{{ t('settings.diffNone') }}</div>
    <div v-else class="difftable">
      <div class="difftable__head">
        <span>{{ t('settings.diffPath') }}</span><span>{{ t('settings.diffChange') }}</span>
        <span>{{ t('settings.diffOldHeader') }}</span><span>{{ t('settings.diffNewHeader') }}</span>
      </div>
      <div v-for="(c, i) in changes" :key="i" class="diffrow" :class="opClass(c.op)">
        <span class="diffrow__path mono">{{ c.path || '—' }}</span>
        <span><span class="diffrow__op">{{ opLabel(c.op) }}</span></span>
        <span class="diffrow__val mono">{{ showVal(c.old ?? (c.op === 'added' ? undefined : c.value)) }}</span>
        <span class="diffrow__val mono">{{ showVal(c.new ?? (c.op === 'added' ? c.value : undefined)) }}</span>
      </div>
    </div>
  </div>
</template>

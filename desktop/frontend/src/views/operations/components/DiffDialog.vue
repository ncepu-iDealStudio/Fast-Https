<script setup lang="ts">
/** 版本差异弹窗：按文件分组展示增删改，避免把整段技术路径铺在页面上。 */
import { computed } from 'vue';
import FhDialog from '../../../components/ui/FhDialog.vue';
import { useI18n } from '../../../composables/useI18n';

interface Change { path?: string; op?: string; value?: unknown; old?: unknown; new?: unknown; changed?: boolean }
const props = defineProps<{
  open: boolean; base?: string; version: string; changes: Change[]; missing?: boolean;
}>();
const emit = defineEmits<{ 'update:open': [v: boolean] }>();
const { t } = useI18n();

interface Row extends Change { file: string; field: string }

const groups = computed<[string, Row[]][]>(() => {
  const map = new Map<string, Row[]>();
  for (const c of props.changes) {
    const p = c.path ?? '';
    const i = p.indexOf(':');
    const row: Row = i < 0
      ? { ...c, file: p, field: '' }
      : { ...c, file: p.slice(0, i), field: p.slice(i + 1) };
    const list = map.get(row.file) ?? [];
    list.push(row);
    map.set(row.file, list);
  }
  return [...map.entries()].sort(([a], [b]) => a.localeCompare(b));
});

function showVal(v: unknown): string {
  if (v === undefined || v === null || v === '') return '—';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}
function fmtVersion(v?: string): string {
  if (!v) return '';
  const m = /^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/.exec(v);
  return m ? `${m[1]}-${m[2]}-${m[3]} ${m[4]}:${m[5]}:${m[6]}` : v;
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
  <FhDialog
    :open="open" :title="t('ops.diffTitle')"
    :description="base ? `${fmtVersion(base)} → ${fmtVersion(version)}` : fmtVersion(version)"
    width="880px"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <p v-if="missing" class="notice">{{ t('ops.noChangesRecorded') }}</p>
    <p v-else-if="!changes.length" class="notice">{{ t('settings.diffNone') }}</p>
    <template v-else>
      <div class="diff-summary">{{ t('ops.diffCount', { n: changes.length }) }}</div>
      <div v-for="[file, list] in groups" :key="file" class="diff-group">
        <div class="diff-group__file mono">{{ file }}</div>
        <div class="difftable">
          <div class="difftable__head">
            <span>{{ t('settings.diffPath') }}</span><span>{{ t('settings.diffChange') }}</span>
            <span>{{ t('settings.diffOldHeader') }}</span><span>{{ t('settings.diffNewHeader') }}</span>
          </div>
          <div v-for="(c, i) in list" :key="i" class="diffrow" :class="opClass(c.op)">
            <span class="diffrow__path mono">{{ c.field || t('ops.wholeFile') }}</span>
            <span>
              <span class="diffrow__op">{{ opLabel(c.op) }}</span>
              <span v-if="c.changed" class="diffrow__masked">{{ t('settings.diffSensitive') }}</span>
            </span>
            <span class="diffrow__val mono">{{ showVal(c.old ?? (c.op === 'added' ? undefined : c.value)) }}</span>
            <span class="diffrow__val mono">{{ showVal(c.new ?? (c.op === 'added' ? c.value : undefined)) }}</span>
          </div>
        </div>
      </div>
    </template>
  </FhDialog>
</template>

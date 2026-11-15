<script setup lang="ts">
/** 单个配置字段：按 fieldSpec.type 选控件，值由父级 values 驱动。 */
import { computed } from 'vue';
import { Plus, X, FolderOpen } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';
import type { ConfigField as Field } from '../../../types/config';

const props = defineProps<{
  field: Field;
  value: string;
}>();
const emit = defineEmits<{
  change: [v: string];
  browse: [];
}>();
const { t } = useI18n();

const value = computed(() => props.value ?? '');

function multiChecked(opt: string): boolean {
  return value.value.split('\n').map((s) => s.trim()).includes(opt);
}
function toggleMulti(opt: string): void {
  const cur = value.value.split('\n').map((s) => s.trim()).filter(Boolean);
  emit('change', (cur.includes(opt) ? cur.filter((x) => x !== opt) : [...cur, opt]).join('\n'));
}
function pathRows(): string[] {
  const rows = value.value.split('\n').map((s) => s.trim()).filter(Boolean);
  return rows.length ? rows : [''];
}
function setPathRow(i: number, v: string): void {
  const rows = pathRows();
  rows[i] = v;
  emit('change', rows.filter(Boolean).join('\n'));
}
function addPathRow(): void {
  const rows = pathRows();
  rows.push('');
  emit('change', rows.filter(Boolean).join('\n'));
}
function delPathRow(i: number): void {
  emit('change', pathRows().filter((_, idx) => idx !== i).filter(Boolean).join('\n'));
}
</script>

<template>
  <div class="cfg-row" :class="{ 'cfg-row--missing': field.missing }">
    <label class="cfg-row__label">
      <span>{{ field.label }}</span>
      <small v-if="field.help">{{ field.help }}</small>
    </label>
    <div class="cfg-row__control">
      <!-- 多选 -->
      <div v-if="field.type === 'multiselect'" class="choice-group">
        <label v-for="o in (field.options ?? [])" :key="o" class="mini-field">
          <input type="checkbox" :checked="multiChecked(o)" @change="toggleMulti(o)" />
          <span>{{ o }}</span>
        </label>
      </div>
      <!-- 开关 -->
      <label v-else-if="field.type === 'bool'" class="mini-field">
        <input type="checkbox" :checked="value === 'true'"
               @change="emit('change', ($event.target as HTMLInputElement).checked ? 'true' : 'false')" />
        <span>{{ t('settings.formOn') }}</span>
      </label>
      <!-- 下拉 -->
      <select v-else-if="field.type === 'select'" :value="value"
              @change="emit('change', ($event.target as HTMLSelectElement).value)">
        <option v-for="o in (field.options ?? [])" :key="o" :value="o">
          {{ o === '' ? t('settings.formUnset') : o }}
        </option>
      </select>
      <!-- 路径列表 -->
      <div v-else-if="field.type === 'pathlist'" class="path-list">
        <div v-for="(row, i) in pathRows()" :key="i" class="path-row">
          <input type="text" :value="row" spellcheck="false"
                 @input="setPathRow(i, ($event.target as HTMLInputElement).value)" />
          <button class="icon-btn" :title="t('settings.browse')" @click="emit('browse')"><FolderOpen :size="13" /></button>
          <button class="icon-btn" :title="t('common.delete')" @click="delPathRow(i)"><X :size="13" /></button>
        </div>
        <button class="icon-btn" :title="t('settings.pathAdd')" @click="addPathRow"><Plus :size="13" /></button>
      </div>
      <!-- 多行 -->
      <textarea v-else-if="field.type === 'list' || field.type === 'textarea'"
                rows="3" spellcheck="false" :value="value"
                @input="emit('change', ($event.target as HTMLTextAreaElement).value)" />
      <!-- 单路径 -->
      <div v-else-if="field.browse" class="inline-fields">
        <input type="text" spellcheck="false" :value="value"
               @input="emit('change', ($event.target as HTMLInputElement).value)" />
        <button class="icon-btn" :title="t('settings.browse')" @click="emit('browse')"><FolderOpen :size="13" /></button>
      </div>
      <!-- 普通文本 / 数字 / 邮箱 -->
      <input v-else
             :type="field.type === 'number' ? 'number' : (field.type === 'email' ? 'email' : 'text')"
             :value="value"
             @input="emit('change', ($event.target as HTMLInputElement).value)" />
    </div>
  </div>
</template>

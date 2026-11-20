<script setup lang="ts">
/** 单个配置字段：按 fieldSpec.type 选控件，值由父级 values 驱动。 */
import { computed, ref, watch } from 'vue';
import { Plus, Minus, X, FolderOpen, CircleAlert } from '@lucide/vue';
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

// 输入示例与加减步长：只影响输入框提示和按钮，不改变实际值。
const PLACEHOLDERS: Record<string, string> = {
  'security.headers.hsts': 'max-age=31536000; includeSubDomains',
  'security.headers.content_security_policy': "default-src 'self'",
  'acme.email': 'admin@example.com',
  'acme.renew_before_days': '30',
  'acme.storage': './config/acme',
  'tls.default_certificate.certificate': './config/cert/default.pem',
  'tls.default_certificate.key': './config/cert/default-key.pem',
  'proxy.trusted_proxies': '127.0.0.1/32',
  'proxy.forwarded_for.max_hops': '10',
  'proxy.upstream_timeout': '20',
  'proxy.tunnel_idle_timeout': '0',
  'http.include': './conf.d',
  'http.default_type': 'application/octet-stream',
  'http.servers_limit.max_header_size': '8192',
  'http.servers_limit.max_body_size': '8388608',
  'http.servers_limit.read_timeout': '30',
  'http.servers_limit.write_timeout': '30',
  'http.servers_limit.body_timeout': '1',
  'http.servers_limit.limit': '0',
  'http.servers_limit.burst': '0',
  'http.blaklist': '203.0.113.10',
};

const STEPS: Record<string, number> = {
  'http.servers_limit.max_header_size': 1024,
  'http.servers_limit.max_body_size': 1048576,
};

const placeholder = computed(() => PLACEHOLDERS[props.field.path] ?? '');
const step = computed(() => STEPS[props.field.path] ?? 1);

function bump(dir: 1 | -1): void {
  const cur = Number.parseInt(value.value || '0', 10);
  const next = Math.max(0, (Number.isFinite(cur) ? cur : 0) + dir * step.value);
  emit('change', String(next));
}

function multiChecked(opt: string): boolean {
  return value.value.split('\n').map((s) => s.trim()).includes(opt);
}
function toggleMulti(opt: string): void {
  const cur = value.value.split('\n').map((s) => s.trim()).filter(Boolean);
  emit('change', (cur.includes(opt) ? cur.filter((x) => x !== opt) : [...cur, opt]).join('\n'));
}
// 逐条编辑的列表（pathlist / list）：行状态留在组件内，否则「+」新增的空行
// 会被父级的过滤逻辑吃掉，点了没反应。lastEmitted 用来区分「自己刚发出的值」
// 与「外部重新读取后的值」，避免保存后把正在编辑的空行清掉。
const rows = ref<string[]>(['']);
const lastEmitted = ref('');

function syncRows(v: string): void {
  if (v === lastEmitted.value) return;
  const list = v.split('\n').map((s) => s.trim()).filter(Boolean);
  rows.value = list.length ? list : [''];
  lastEmitted.value = v;
}

watch(() => props.value, (v) => syncRows(v ?? ''), { immediate: true });

function emitRows(): void {
  const joined = rows.value.map((s) => s.trim()).filter(Boolean).join('\n');
  lastEmitted.value = joined;
  emit('change', joined);
}
function setRow(i: number, v: string): void {
  rows.value[i] = v;
  emitRows();
}
function addRow(): void {
  rows.value.push('');
}
function delRow(i: number): void {
  rows.value.splice(i, 1);
  if (rows.value.length === 0) rows.value = [''];
  emitRows();
}
</script>

<template>
  <div class="cfg-row" :class="{ 'cfg-row--missing': field.missing }">
    <label class="cfg-row__label">
      <span class="cfg-row__title">
        <span>{{ field.label }}</span>
        <!-- 未配置只用一个图标示意：这一列几乎每行都缺省，铺满「未配置」
             文字反而把真正要看的字段淹掉。语义放在 title/aria 上。 -->
        <span
          v-if="field.missing"
          class="cfg-row__missing"
          role="img"
          :aria-label="t('settings.formMissing')"
          :title="t('settings.formMissing')"
        >
          <CircleAlert :size="13" />
        </span>
      </span>
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
        <div v-for="(row, i) in rows" :key="i" class="path-row">
          <input type="text" :value="row" spellcheck="false" :placeholder="placeholder"
                 @input="setRow(i, ($event.target as HTMLInputElement).value)" />
          <button class="icon-btn" :title="t('settings.browse')" :aria-label="t('settings.browse')" @click="emit('browse')"><FolderOpen :size="13" /></button>
          <button class="icon-btn" :title="t('common.delete')" :aria-label="t('common.delete')" @click="delRow(i)"><X :size="13" /></button>
        </div>
        <button class="icon-btn" :title="t('settings.pathAdd')" :aria-label="t('settings.pathAdd')" @click="addRow"><Plus :size="13" /></button>
      </div>
      <!-- 逐条列表：黑名单、可信代理，一条一个输入框 -->
      <div v-else-if="field.type === 'list'" class="path-list">
        <div v-for="(row, i) in rows" :key="i" class="path-row">
          <input type="text" :value="row" spellcheck="false" :placeholder="placeholder"
                 @input="setRow(i, ($event.target as HTMLInputElement).value)" />
          <button class="icon-btn" :title="t('common.delete')" :aria-label="t('common.delete')" @click="delRow(i)"><X :size="13" /></button>
        </div>
        <button class="icon-btn" :title="t('settings.listAdd')" :aria-label="t('settings.listAdd')" @click="addRow"><Plus :size="13" /></button>
      </div>
      <!-- 多行 -->
      <textarea v-else-if="field.type === 'textarea'"
                rows="3" spellcheck="false" :value="value" :placeholder="placeholder"
                @input="emit('change', ($event.target as HTMLTextAreaElement).value)" />
      <!-- 数字：加减按钮 + 示例占位 -->
      <div v-else-if="field.type === 'number'" class="num-field">
        <button class="icon-btn" :title="t('common.decrease')" :aria-label="t('common.decrease')" @click="bump(-1)"><Minus :size="13" /></button>
        <input type="number" min="0" :step="step" :placeholder="placeholder" :value="value"
               @input="emit('change', ($event.target as HTMLInputElement).value)" />
        <button class="icon-btn" :title="t('common.increase')" :aria-label="t('common.increase')" @click="bump(1)"><Plus :size="13" /></button>
      </div>
      <!-- 单路径 -->
      <div v-else-if="field.browse" class="inline-fields">
        <input type="text" spellcheck="false" :value="value" :placeholder="placeholder"
               @input="emit('change', ($event.target as HTMLInputElement).value)" />
        <button class="icon-btn" :title="t('settings.browse')" :aria-label="t('settings.browse')" @click="emit('browse')"><FolderOpen :size="13" /></button>
      </div>
      <!-- 普通文本 / 邮箱 -->
      <input v-else
             :type="field.type === 'email' ? 'email' : 'text'"
             :value="value" :placeholder="placeholder"
             @input="emit('change', ($event.target as HTMLInputElement).value)" />
    </div>
  </div>
</template>

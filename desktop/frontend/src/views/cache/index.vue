<script setup lang="ts">
/** 缓存区页：主配置 cache 段的增删改查，全部转交 CLI。 */
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { FolderOpen, Pencil, Plus, Trash2 } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { quipOfDay } from '../../content/quips';
import { listCacheZones, addCacheZone, saveCacheZone, deleteCacheZone } from '../../services/api/cache';
import { messageOf } from '../../services/api/base';
import { pickPath } from '../../services/api/app';
import FhButton from '../../components/ui/FhButton.vue';
import PageTitle from '../../components/common/PageTitle.vue';
import FhDialog from '../../components/ui/FhDialog.vue';
import ConfirmDialog from '../../components/common/ConfirmDialog.vue';
import type { CacheZone } from '../../services/api/types';

const { t, lang } = useI18n();
const quip = computed(() => quipOfDay(lang()));

const zones = ref<CacheZone[]>([]);
const loading = ref(false);
const error = ref('');
const notice = ref('');

// 新增 / 编辑共用同一个对话框；editingName 为空表示新增。
const dialogOpen = ref(false);
const editingName = ref('');
const saving = ref(false);
const formError = ref('');
const form = reactive({ name: '', path: '', maxSize: 0 });
// 用户手动改过目录后，不再让名称联动覆盖它。
const pathTouched = ref(false);

watch(() => form.name, (name) => {
  if (editingName.value || pathTouched.value) return;
  const n = name.trim();
  form.path = n ? `./.cache/${n}` : '';
});

const delOpen = ref(false);
const delName = ref('');
const delBusy = ref(false);

// 有效期在界面上按「状态码 + 时长 + 单位」编辑，提交时才拼成 any:30s / 200:10m。
type Unit = 's' | 'm' | 'h' | 'd';
interface ValidRule { status: string; amount: number; unit: Unit }
const STATUS_OPTIONS = ['any', '200', '204', '301', '302', '304', '400', '401', '403', '404', '500', '502', '503', '504'];
const UNIT_SECONDS: Record<Unit, number> = { s: 1, m: 60, h: 3600, d: 86400 };
const validRules = ref<ValidRule[]>(defaultRules());

function defaultRules(): ValidRule[] {
  return [{ status: 'any', amount: 10, unit: 'm' }];
}

// 已有配置里可能出现下拉列表之外的精确状态码，回填时补进选项。
const statusOptions = computed(() => {
  const out = [...STATUS_OPTIONS];
  for (const rule of validRules.value) {
    if (rule.status && !out.includes(rule.status)) out.push(rule.status);
  }
  return out;
});

// parseDuration 接受 30s / 10m / 2h / 7d，也兼容 1h30m 这类组合写法。
function parseDuration(raw: string): { amount: number; unit: Unit } | null {
  const simple = /^(\d+)([smhd])$/.exec(raw.trim());
  if (simple) {
    const amount = Number(simple[1]);
    if (amount > 0) return { amount, unit: simple[2] as Unit };
  }
  const m = /^(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(raw.trim());
  if (!m) return null;
  const secs = Number(m[1] ?? 0) * UNIT_SECONDS.d + Number(m[2] ?? 0) * UNIT_SECONDS.h +
    Number(m[3] ?? 0) * UNIT_SECONDS.m + Number(m[4] ?? 0) * UNIT_SECONDS.s;
  if (secs <= 0) return null;
  if (secs % UNIT_SECONDS.d === 0) return { amount: secs / UNIT_SECONDS.d, unit: 'd' };
  if (secs % UNIT_SECONDS.h === 0) return { amount: secs / UNIT_SECONDS.h, unit: 'h' };
  if (secs % UNIT_SECONDS.m === 0) return { amount: secs / UNIT_SECONDS.m, unit: 'm' };
  return { amount: secs, unit: 's' };
}

function rulesFromValid(valid: string[]): ValidRule[] {
  const rules: ValidRule[] = [];
  for (const item of valid ?? []) {
    const idx = item.indexOf(':');
    if (idx < 0) continue;
    const status = item.slice(0, idx).trim();
    const duration = parseDuration(item.slice(idx + 1));
    if (status && duration) rules.push({ status, ...duration });
  }
  return rules.length ? rules : defaultRules();
}

function unitLabel(unit: Unit): string {
  return t(`cache.unit.${unit}`);
}

function durationText(raw: string): string {
  const parsed = parseDuration(raw);
  return parsed ? `${parsed.amount} ${unitLabel(parsed.unit)}` : raw;
}

// 列表里把 any:30s 显示成「全部状态 30 秒」，不再暴露原始写法。
function humanizeValid(valid: string[]): string {
  if (!valid?.length) return '—';
  return valid.map((item) => {
    const idx = item.indexOf(':');
    const status = (idx < 0 ? item : item.slice(0, idx)).trim();
    const duration = idx < 0 ? '' : item.slice(idx + 1).trim();
    const label = status === 'any' ? t('cache.validAny') : status;
    return duration ? `${label} ${durationText(duration)}` : label;
  }).join(' · ');
}

function addRule(): void {
  validRules.value.push({ status: '200', amount: 30, unit: 'm' });
}

function removeRule(index: number): void {
  if (validRules.value.length > 1) validRules.value.splice(index, 1);
}

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    zones.value = await listCacheZones();
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    loading.value = false;
  }
}

function openAdd(): void {
  editingName.value = '';
  form.name = '';
  form.path = '';
  form.maxSize = 0;
  pathTouched.value = false;
  validRules.value = defaultRules();
  formError.value = '';
  dialogOpen.value = true;
}

function openEdit(z: CacheZone): void {
  editingName.value = z.name;
  form.name = z.name;
  form.path = z.path;
  form.maxSize = z.max_size ?? 0;
  pathTouched.value = true;
  validRules.value = rulesFromValid(z.valid);
  formError.value = '';
  dialogOpen.value = true;
}

async function browsePath(): Promise<void> {
  try {
    const p = await pickPath('dir');
    if (p) {
      form.path = p;
      pathTouched.value = true;
    }
  } catch (e) {
    formError.value = messageOf(e);
  }
}

function toInt(v: number | string): number {
  const n = Number(v);
  return Number.isFinite(n) ? Math.trunc(n) : 0;
}

async function submit(): Promise<void> {
  const name = form.name.trim();
  const dir = form.path.trim();
  if (!name || !dir) {
    formError.value = t('cache.invalid');
    return;
  }
  const valid: string[] = [];
  const seen = new Set<string>();
  for (const rule of validRules.value) {
    const amount = Math.trunc(Number(rule.amount));
    if (!rule.status || !Number.isFinite(amount) || amount <= 0) {
      formError.value = t('cache.validRequired');
      return;
    }
    if (seen.has(rule.status)) {
      formError.value = t('cache.validDuplicate');
      return;
    }
    seen.add(rule.status);
    valid.push(`${rule.status}:${amount}${rule.unit}`);
  }
  if (!valid.length) {
    formError.value = t('cache.validRequired');
    return;
  }
  const zone: CacheZone = { name, path: dir, valid, max_size: Math.max(0, toInt(form.maxSize)) };
  saving.value = true;
  formError.value = '';
  error.value = '';
  notice.value = '';
  try {
    notice.value = editingName.value
      ? await saveCacheZone(editingName.value, zone)
      : await addCacheZone(zone);
    dialogOpen.value = false;
    await load();
  } catch (e) {
    formError.value = messageOf(e);
  } finally {
    saving.value = false;
  }
}

function askDelete(name: string): void {
  delName.value = name;
  delOpen.value = true;
}

async function doDelete(): Promise<void> {
  const name = delName.value;
  if (!name) return;
  delBusy.value = true;
  error.value = '';
  notice.value = '';
  try {
    notice.value = await deleteCacheZone(name);
    delOpen.value = false;
    await load();
  } catch (e) {
    error.value = messageOf(e);
    delOpen.value = false;
  } finally {
    delBusy.value = false;
    delName.value = '';
  }
}

onMounted(() => { void load(); });
</script>

<template>
  <PageTitle :title="t('nav.cache')">
    <span class="page-title__actions">
      <FhButton variant="primary" @click="openAdd">
        <Plus :size="14" /><span>{{ t('cache.add') }}</span>
      </FhButton>
    </span>
  </PageTitle>

  <div class="main__body">
    <p v-if="error" class="notice notice--err">{{ error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <div class="table">
      <div class="table__head">
        <span>{{ t('cache.name') }}</span>
        <span>{{ t('cache.path') }}</span>
        <span>{{ t('cache.valid') }}</span>
        <span />
      </div>
      <div v-for="z in zones" :key="z.name" class="table__row">
        <span class="mono ellipsis">{{ z.name }}</span>
        <span class="mono ellipsis">{{ z.path }}</span>
        <span class="ellipsis">
          {{ humanizeValid(z.valid) }}
          <small v-if="z.max_size > 0" class="muted"> · {{ z.max_size }} B</small>
        </span>
        <span class="table__ops">
          <button class="icon-btn" :title="t('cache.edit')" @click="openEdit(z)"><Pencil :size="14" /></button>
          <button class="icon-btn icon-btn--danger" :title="t('cache.delete')" @click="askDelete(z.name)">
            <Trash2 :size="14" />
          </button>
        </span>
      </div>
      <div v-if="!zones.length && !loading" class="table__empty">
        {{ t('cache.empty') }}
        <span class="table__quip">{{ quip }}</span>
      </div>
    </div>

    <FhDialog
      v-model:open="dialogOpen"
      :title="editingName ? t('cache.editTitle') : t('cache.addTitle')"
      :description="t('cache.dialogHint')"
      width="520px"
    >
      <div class="cache-form adv__fields adv__fields--2">
        <label class="stack-field span-2">
          <span>{{ t('cache.name') }}</span>
          <input v-model="form.name" type="text" :placeholder="t('cache.namePh')" :disabled="!!editingName" />
        </label>
        <label class="stack-field span-2">
          <span>{{ t('cache.path') }}</span>
          <div class="joined">
            <input v-model="form.path" type="text" :placeholder="t('cache.pathPh')" @input="pathTouched = true" />
            <FhButton size="small" @click="browsePath"><FolderOpen :size="14" /><span>{{ t('cache.browse') }}</span></FhButton>
          </div>
        </label>
        <div class="stack-field span-2">
          <span>{{ t('cache.valid') }}</span>
          <div v-for="(rule, i) in validRules" :key="i" class="valid-row">
            <select v-model="rule.status" :title="t('cache.validStatus')">
              <option v-for="s in statusOptions" :key="s" :value="s">
                {{ s === 'any' ? t('cache.validAny') : s }}
              </option>
            </select>
            <input v-model.number="rule.amount" type="number" min="1" :title="t('cache.validAmount')" />
            <select v-model="rule.unit" :title="t('cache.validUnit')">
              <option value="s">{{ t('cache.unit.s') }}</option>
              <option value="m">{{ t('cache.unit.m') }}</option>
              <option value="h">{{ t('cache.unit.h') }}</option>
              <option value="d">{{ t('cache.unit.d') }}</option>
            </select>
            <button
              type="button" class="icon-btn icon-btn--danger"
              :title="t('cache.validRemove')" :disabled="validRules.length === 1"
              @click="removeRule(i)"
            ><Trash2 :size="14" /></button>
          </div>
          <button type="button" class="valid-add" @click="addRule">{{ t('cache.validAdd') }}</button>
        </div>
        <label class="stack-field">
          <span>{{ t('cache.maxSize') }}</span>
          <input v-model.number="form.maxSize" type="number" min="0" />
        </label>
        <small v-if="editingName" class="field-note span-2">{{ t('cache.nameLocked') }}</small>
      </div>
      <p v-if="formError" class="notice notice--err">{{ formError }}</p>
      <template #footer>
        <FhButton @click="dialogOpen = false">{{ t('common.cancel') }}</FhButton>
        <FhButton variant="primary" :loading="saving" @click="submit">{{ t('cache.save') }}</FhButton>
      </template>
    </FhDialog>

    <ConfirmDialog
      v-model:open="delOpen"
      :title="t('cache.delTitle')"
      :message="t('cache.delBody', { name: delName })"
      :loading="delBusy"
      @confirm="doDelete"
    />
  </div>
</template>

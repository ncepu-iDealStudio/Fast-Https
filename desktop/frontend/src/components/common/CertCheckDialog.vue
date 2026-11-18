<script setup lang="ts">
/**
 * 证书检查弹窗：cert check --domain <域名>（§6.2）的结果展示。
 * passed 大字 + exists/validity/hostname/key_pair/chain 逐项检查。
 * 检查结果是数据（证书有问题也退出 0）；域名不在任何 ssl 站点 → NOT_FOUND。
 */
import { computed, ref, watch } from 'vue';
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';

const props = defineProps<{ open: boolean; domain: string }>();
const emit = defineEmits<{ 'update:open': [v: boolean] }>();
const { t } = useI18n();

interface CertCheckItem { checked: boolean; ok: boolean; detail: string }
interface CertChainItem { checked: boolean; certificates: number }
interface CertCheckData {
  domain: string;
  passed: boolean;
  checks: Record<string, CertCheckItem | CertChainItem>;
}

/** 5 项检查 → 字典 key，顺序固定（chain 单独处理）。 */
const CHECK_ITEMS: { key: string; label: string }[] = [
  { key: 'exists', label: 'dialog.certcheck.iExists' },
  { key: 'validity', label: 'dialog.certcheck.iValidity' },
  { key: 'hostname', label: 'dialog.certcheck.iHostname' },
  { key: 'key_pair', label: 'dialog.certcheck.iKeyPair' },
  { key: 'chain', label: 'dialog.certcheck.iChain' },
];

const busy = ref(false);
const error = ref('');
const data = ref<CertCheckData | null>(null);
/** 请求失败：大字改为「检查失败」，与「未通过」区分。 */
const failed = ref(false);

function text(v: unknown): string {
  if (v === null || v === undefined) return '';
  return typeof v === 'string' ? v : String(v);
}

/** 把 CLI 的 data 收窄成本组件要的严格结构（不使用 any）。 */
function parseData(value: unknown): CertCheckData {
  const o = (value ?? {}) as Record<string, unknown>;
  const raw = (o.checks ?? {}) as Record<string, unknown>;
  const checks: Record<string, CertCheckItem | CertChainItem> = {};
  for (const { key } of CHECK_ITEMS) {
    const c = (raw[key] ?? {}) as Record<string, unknown>;
    if (key === 'chain') {
      const n = Number(c.certificates_in_file ?? 0);
      checks[key] = { checked: !!c.checked, certificates: Number.isFinite(n) ? n : 0 };
    } else {
      checks[key] = { checked: !!c.checked, ok: !!c.ok, detail: text(c.detail) };
    }
  }
  return { domain: text(o.domain), passed: !!o.passed, checks };
}

const domainText = computed(() => data.value?.domain || props.domain || '');

const passedClass = computed(() => {
  if (failed.value) return 'diag-overall-text st-err';
  if (!data.value) return 'diag-overall-text';
  return data.value.passed ? 'diag-overall-text st-ok' : 'diag-overall-text st-err';
});
const passedText = computed(() => {
  if (failed.value) return t('dialog.certcheck.checkFailed');
  if (!data.value) return busy.value ? t('dialog.certcheck.checking') : '—';
  return data.value.passed ? t('dialog.certcheck.passed') : t('dialog.certcheck.failed');
});

/** 单项状态：chain 恒为「未验证」，其余按 checked/ok 区分「未检查 / 通过 / 未通过」。 */
function stateOf(key: string): { cls: string; label: string } {
  if (key === 'chain') return { cls: 'badge--off', label: t('dialog.certcheck.stNotVerified') };
  const c = data.value?.checks[key] as CertCheckItem | undefined;
  if (!c?.checked) return { cls: 'badge--off', label: t('dialog.certcheck.stNotChecked') };
  return c.ok
    ? { cls: 'badge--ok', label: t('dialog.certcheck.stPass') }
    : { cls: 'badge--err', label: t('dialog.certcheck.stFail') };
}

/** 细则行：chain 显示文件内证书张数，其余显示 Go 端给的 detail。 */
function detailOf(key: string): string {
  const c = data.value?.checks[key];
  if (!c) return '—';
  if (key === 'chain') {
    const n = (c as CertChainItem).certificates;
    return n > 0 ? t('dialog.certcheck.chainCount', { n }) : '—';
  }
  return (c as CertCheckItem).detail || '—';
}

async function check(): Promise<void> {
  if (busy.value || !props.domain) return;
  busy.value = true;
  error.value = '';
  failed.value = false;
  data.value = null;
  try {
    const res = await runCommand(['cert', 'check', '--domain', props.domain]);
    data.value = parseData(res.data);
  } catch (e) {
    failed.value = true;
    error.value = messageOf(e);
  } finally {
    busy.value = false;
  }
}

watch(() => props.open, (open) => {
  if (!open) return;
  error.value = '';
  failed.value = false;
  data.value = null;
  void check();
}, { immediate: true });
</script>

<template>
  <FhDialog
    :open="open" :title="t('dialog.certcheck.title')" width="560px"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <div class="diag-overall">
      <strong :class="passedClass">{{ passedText }}</strong>
      <span v-if="domainText" class="muted mono">{{ domainText }}</span>
    </div>

    <div class="kv-table">
      <div v-if="!data" class="kv-row">
        <span class="k">—</span>
        <span class="v">{{ busy ? t('dialog.certcheck.checking') : t('dialog.certcheck.notChecked') }}</span>
      </div>
      <template v-else>
        <div v-for="item in CHECK_ITEMS" :key="item.key" class="kv-row kv-row--status">
          <span class="k">{{ t(item.label) }}</span>
          <span class="v">{{ detailOf(item.key) }}</span>
          <span class="n">
            <span class="badge" :class="stateOf(item.key).cls">{{ stateOf(item.key).label }}</span>
          </span>
        </div>
      </template>
    </div>

    <p v-if="error" class="dialog-error">{{ error }}</p>

    <template #footer>
      <FhButton variant="primary" @click="emit('update:open', false)">
        {{ t('common.close') }}
      </FhButton>
    </template>
  </FhDialog>
</template>

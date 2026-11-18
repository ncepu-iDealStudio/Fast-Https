<script setup lang="ts">
/**
 * 健康检查弹窗：diagnose 命令（§5.3）的结果展示。
 * overall 大字 + 固定 9 项检查（name/status/message）+ report_id。
 * 诊断本身是成功的查询（退出码 0），overall 由 Go 端汇总，前端只做映射与着色。
 */
import { computed, ref, watch } from 'vue';
import { RefreshCw } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { runCommand } from '../../services/api/command';
import { messageOf } from '../../services/api/base';

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ 'update:open': [v: boolean] }>();
const { t } = useI18n();

interface DiagCheck { name: string; status: string; message: string }
interface DiagReport { overall: string; reportId: string; checks: DiagCheck[] }
interface DiagRow { name: string; raw: string; message: string; cls: string; statusText: string }

/** 9 项检查名 → 字典 key（顺序与 Go 端固定一致，未知项回退原始 name）。 */
const CHECK_NAMES: Record<string, string> = {
  service: 'dialog.diag.nService',
  config: 'dialog.diag.nConfig',
  config_drift: 'dialog.diag.nConfigDrift',
  certs: 'dialog.diag.nCerts',
  ports: 'dialog.diag.nPorts',
  logs: 'dialog.diag.nLogs',
  runtime: 'dialog.diag.nRuntime',
  monitor: 'dialog.diag.nMonitor',
  state: 'dialog.diag.nState',
};

/** 4 种检查状态 → 徽标修饰类 + 字典 key。 */
const CHECK_STATUS: Record<string, { cls: string; key: string }> = {
  ok: { cls: 'badge--ok', key: 'dialog.diag.stOk' },
  warning: { cls: 'badge--warn', key: 'dialog.diag.stWarning' },
  error: { cls: 'badge--err', key: 'dialog.diag.stError' },
  unavailable: { cls: 'badge--off', key: 'dialog.diag.stUnavailable' },
};

/** 整体结论 → 大字着色类 + 字典 key。 */
const OVERALL: Record<string, { cls: string; key: string }> = {
  ok: { cls: 'st-ok', key: 'dialog.diag.allOk' },
  warning: { cls: 'st-warn', key: 'dialog.diag.hasWarning' },
  error: { cls: 'st-err', key: 'dialog.diag.hasError' },
};

const busy = ref(false);
const error = ref('');
const report = ref<DiagReport | null>(null);

const rows = computed<DiagRow[]>(() => (report.value?.checks ?? []).map((c) => {
  const st = CHECK_STATUS[c.status] ?? { cls: 'badge--off', key: '' };
  return {
    name: CHECK_NAMES[c.name] ? t(CHECK_NAMES[c.name]) : c.name,
    raw: c.name,
    message: c.message || '—',
    cls: st.cls,
    statusText: st.key ? t(st.key) : (c.status || '—'),
  };
}));

const overallClass = computed(() => {
  const o = OVERALL[report.value?.overall ?? ''];
  return o ? `diag-overall-text ${o.cls}` : 'diag-overall-text';
});
const overallText = computed(() => {
  const o = OVERALL[report.value?.overall ?? ''];
  if (o) return t(o.key);
  return report.value?.overall || (busy.value ? t('dialog.diag.checking') : '—');
});

function text(v: unknown): string {
  if (v === null || v === undefined) return '';
  return typeof v === 'string' ? v : String(v);
}

/** 把 CLI 的 data 收窄成本组件要的严格结构（不使用 any）。 */
function parseReport(value: unknown): DiagReport {
  const o = (value ?? {}) as Record<string, unknown>;
  const raw = Array.isArray(o.checks) ? o.checks : [];
  return {
    overall: text(o.overall),
    reportId: text(o.report_id),
    checks: raw.map((item) => {
      const c = (item ?? {}) as Record<string, unknown>;
      return { name: text(c.name), status: text(c.status), message: text(c.message) };
    }),
  };
}

async function refresh(): Promise<void> {
  if (busy.value) return;
  busy.value = true;
  error.value = '';
  try {
    const res = await runCommand(['diagnose']);
    report.value = parseReport(res.data);
  } catch (e) {
    report.value = null;
    error.value = messageOf(e);
  } finally {
    busy.value = false;
  }
}

watch(() => props.open, (open) => {
  if (!open) return;
  report.value = null;
  error.value = '';
  void refresh();
}, { immediate: true });
</script>

<template>
  <FhDialog
    :open="open" :title="t('dialog.diag.title')" width="620px"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <div class="diag-overall">
      <span class="diag-label">{{ t('dialog.diag.overall') }}</span>
      <strong :class="overallClass">{{ overallText }}</strong>
      <span v-if="report?.reportId" class="muted mono">report_id {{ report.reportId }}</span>
    </div>

    <div class="kv-table">
      <div v-if="!rows.length" class="kv-row">
        <span class="k">—</span>
        <span class="v">
          {{ busy ? t('dialog.diag.checking') : error ? t('dialog.diag.failed') : t('dialog.diag.notChecked') }}
        </span>
      </div>
      <template v-else>
        <div v-for="row in rows" :key="row.raw" class="kv-row kv-row--status">
          <span class="k">{{ row.name }}</span>
          <span class="v">{{ row.message }}</span>
          <span class="n">
            <span class="badge" :class="row.cls" :title="row.raw">{{ row.statusText }}</span>
            <small class="muted mono">{{ row.raw }}</small>
          </span>
        </div>
      </template>
    </div>

    <p v-if="error" class="dialog-error">{{ error }}</p>

    <template #footer>
      <FhButton :loading="busy" @click="refresh">
        <RefreshCw :size="14" /><span>{{ t('dialog.diag.refresh') }}</span>
      </FhButton>
      <FhButton variant="primary" @click="emit('update:open', false)">
        {{ t('common.close') }}
      </FhButton>
    </template>
  </FhDialog>
</template>

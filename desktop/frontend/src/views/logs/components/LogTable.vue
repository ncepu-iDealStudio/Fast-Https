<script setup lang="ts">
/** 日志表格：结构化七列，状态色编码，滚到底自动续拉。 */
import { ChevronDown } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';
import type { LogEntry } from '../../../types/model';

defineProps<{ rows: LogEntry[]; loading: boolean; hasMore: boolean }>();
const emit = defineEmits<{ scroll: [e: Event]; more: [] }>();
const { t } = useI18n();

function timeText(s?: string): string {
  const m = String(s || '').match(/T(\d{2}:\d{2}:\d{2})(?:\.(\d{1,3}))?/);
  return m ? m[1] + (m[2] ? '.' + m[2] : '') : (s || '—');
}
function tone(e: LogEntry): 'ok' | 'warn' | 'err' | '' {
  const code = Number(String(e.status || '').replace(/\D.*$/, ''));
  if (!code) return e.level === 'error' ? 'err' : '';
  if (code >= 500) return 'err';
  if (code >= 400) return 'warn';
  return 'ok';
}
// 拒绝原因、命中的规则与客户端 IP 压在一格里；完整内容交给 title。
// 旧日志没有 reject_reason 时回退到安全日志的 reason。
// 表格展示的是日志原始值，不翻译；中文只用于筛选框选项。
function reasonCode(e: LogEntry): string {
  return e.rejectReason || e.reason || '';
}
function reasonText(e: LogEntry): string {
  const code = reasonCode(e);
  if (!code) return '—';
  return e.rule ? `${code} · ${e.rule}` : code;
}
function reasonTitle(e: LogEntry): string {
  return [reasonCode(e), e.rule, e.remoteIp].filter(Boolean).join(' · ');
}
// 级别只染字体颜色：debug 灰、info 绿、warn 橙、error 红。
function levelClass(level?: string): string {
  return level ? `logrow__lvl--${level.toLowerCase()}` : '';
}
</script>

<template>
  <div class="logtable">
    <div class="logtable__head">
      <span>{{ t('logs.colTime') }}</span><span>{{ t('logs.colLevel') }}</span><span>{{ t('logs.colMethod') }}</span>
      <span>{{ t('logs.colPath') }}</span><span>{{ t('logs.colReason') }}</span><span>{{ t('logs.colStatus') }}</span>
      <span>{{ t('logs.colSize') }}</span><span>{{ t('logs.colHost') }} / {{ t('logs.colRemote') }}</span>
    </div>
    <div class="logtable__body" @scroll.passive="emit('scroll', $event)">
      <div v-if="!rows.length && !loading" class="table__empty">{{ t('logs.empty') }}</div>
      <div
        v-for="(e, i) in rows" :key="`${e.time}-${i}`"
        class="logrow" :class="`logrow--${tone(e)}`" :title="e.raw || e.message"
      >
        <span class="logrow__time mono">{{ timeText(e.time) }}</span>
        <span class="logrow__lvl mono" :class="levelClass(e.level)" :title="e.level">{{ e.level || '—' }}</span>
        <span class="logrow__method mono">{{ e.method || '—' }}</span>
        <span class="logrow__path mono">{{ e.path || e.message || '—' }}</span>
        <span class="logrow__reason mono" :class="{ 'logrow__reason--hit': e.rejectReason }" :title="reasonTitle(e)">
          {{ reasonText(e) }}
        </span>
        <span class="logrow__status mono">{{ e.status || '—' }}</span>
        <span class="logrow__size mono">{{ e.size || '—' }}</span>
        <span class="logrow__host mono">{{ e.host || '—' }}<template v-if="e.remoteAddr"> · {{ e.remoteAddr }}</template><template v-else-if="e.remoteIp"> · {{ e.remoteIp }}</template></span>
      </div>
      <div v-if="hasMore" class="logtable__more">
        <button class="icon-btn" :title="t('logs.more')" :disabled="loading" @click="emit('more')">
          <ChevronDown :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>

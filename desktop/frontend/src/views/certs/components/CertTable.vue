<script setup lang="ts">
/** 证书清单表格：一行一张证书，状态徽标 + 到期 + 检查按钮。 */
import { Stethoscope } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';
import type { Cert } from '../../../types/model';

defineProps<{ items: Cert[]; loading: boolean }>();
const emit = defineEmits<{ check: [domain: string] }>();
const { t } = useI18n();

const STATUS_KEY: Record<string, string> = {
  valid: 'certs.stValid', expiring: 'certs.stExpiring',
  expired: 'certs.stExpired', not_yet_valid: 'certs.stNotYetValid',
  missing: 'certs.stMissing', unreadable: 'certs.stUnreadable',
};
function statusLabel(s: string): string {
  return t(STATUS_KEY[s] ?? 'certs.stValid');
}
</script>

<template>
  <div class="table">
    <div class="table__head table__head--certs">
      <span>{{ t('certs.configured') }}</span>
      <span>{{ t('common.status') }}</span>
      <span>expires</span>
      <span class="table__ops" />
    </div>
    <div v-if="!items.length && !loading" class="table__empty">{{ t('certs.empty') }}</div>
    <div v-for="c in items" :key="c.file" class="table__row table__row--certs">
      <span class="mono ellipsis" :title="c.file">
        {{ c.domain }}
        <span v-if="c.selfSigned" class="muted"> · {{ t('certs.selfSigned') }}</span>
        <span v-else-if="c.local" class="muted"> · {{ t('certs.localGenerated') }}</span>
      </span>
      <span>
        <span class="badge" :class="c.status === 'valid' ? 'badge--ok' : 'badge--off'">
          {{ statusLabel(c.status) }}
        </span>
      </span>
      <span class="muted mono">
        {{ c.notAfter || '—' }}
        <template v-if="c.daysLeft > 0"> · {{ t('certs.daysLeft', { days: c.daysLeft }) }}</template>
      </span>
      <span class="table__ops">
        <button class="icon-btn" :title="t('certs.check')" @click="emit('check', c.domain)">
          <Stethoscope :size="14" />
        </button>
      </span>
    </div>
  </div>
</template>

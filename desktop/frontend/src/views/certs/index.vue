<script setup lang="ts">
import { onMounted } from 'vue';
import { RefreshCw, ShieldCheck } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useCertsStore } from '../../stores/certs';
import FhButton from '../../components/ui/FhButton.vue';

const { t } = useI18n();
const certs = useCertsStore();

onMounted(() => { void certs.refresh(); });

function statusLabel(status: string): string {
  const map: Record<string, string> = {
    valid: 'certs.stValid',
    expiring: 'certs.stExpiring',
    expired: 'certs.stExpired',
    not_yet_valid: 'certs.stNotYetValid',
  };
  return t(map[status] ?? 'certs.stValid');
}
</script>

<template>
  <div class="main__body">
    <p class="lede" v-html="t('certs.lede')" />

    <div class="toolbar">
      <FhButton :loading="certs.loading" @click="certs.refresh()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <FhButton>
        <ShieldCheck :size="14" /><span>{{ t('certs.trust') }}</span>
      </FhButton>
    </div>

    <p v-if="certs.error" class="notice notice--err">{{ certs.error }}</p>
    <div v-if="certs.loading && !certs.items.length" class="notice">{{ t('certs.loading') }}</div>

    <div class="table">
      <div class="table__head">
        <span>{{ t('certs.configured') }}</span>
        <span>{{ t('common.status') }}</span>
        <span>expires</span>
      </div>
      <div v-if="!certs.items.length && !certs.loading" class="table__empty">
        {{ t('certs.empty') }}
      </div>
      <div v-for="c in certs.items" :key="c.file" class="table__row">
        <span class="mono ellipsis" :title="c.file">
          {{ c.domain }}
          <small v-if="c.selfSigned" class="muted"> · {{ t('certs.selfSigned') }}</small>
        </span>
        <span>
          <span class="badge" :class="c.daysLeft > 30 ? 'badge--ok' : 'badge--off'">
            {{ statusLabel(c.status) }}
          </span>
        </span>
        <span class="muted">{{ c.notAfter }}<template v-if="c.daysLeft > 0"> · {{ t('certs.daysLeft', { days: c.daysLeft }) }}</template></span>
      </div>
    </div>
  </div>
</template>

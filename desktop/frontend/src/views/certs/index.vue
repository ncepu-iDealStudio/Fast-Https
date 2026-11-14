<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { RefreshCw, ShieldCheck, Stethoscope } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useCertsStore } from '../../stores/certs';
import FhButton from '../../components/ui/FhButton.vue';
import TrustDialog from '../../components/common/TrustDialog.vue';
import CertCheckDialog from '../../components/common/CertCheckDialog.vue';

const { t } = useI18n();
const certs = useCertsStore();
const trustOpen = ref(false);
const checkOpen = ref(false);
const checkDomain = ref('');

function openCheck(domain: string): void {
  checkDomain.value = domain;
  checkOpen.value = true;
}

onMounted(() => { void certs.refresh(); });

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
  <div class="main__body">
    <p class="lede" v-html="t('certs.lede')" />

    <div class="toolbar">
      <FhButton :loading="certs.loading" @click="certs.refresh()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <FhButton @click="trustOpen = true">
        <ShieldCheck :size="14" /><span>{{ t('certs.trust') }}</span>
      </FhButton>
    </div>

    <p class="field-note" v-html="t('certs.note')" />

    <p v-if="certs.error" class="notice notice--err">{{ certs.error }}</p>
    <div v-if="certs.loading && !certs.items.length" class="notice">{{ t('certs.loading') }}</div>

    <div class="table">
      <div class="table__head table__head--certs">
        <span>{{ t('certs.configured') }}</span>
        <span>{{ t('common.status') }}</span>
        <span>expires</span>
        <span class="table__ops" />
      </div>
      <div v-if="!certs.items.length && !certs.loading" class="table__empty">
        {{ t('certs.empty') }}
      </div>
      <div v-for="c in certs.items" :key="c.file" class="table__row table__row--certs">
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
          <button class="icon-btn" :title="t('certs.check')" @click="openCheck(c.domain)">
            <Stethoscope :size="14" />
          </button>
        </span>
      </div>
    </div>

    <TrustDialog v-model:open="trustOpen" />
    <CertCheckDialog v-model:open="checkOpen" :domain="checkDomain" />
  </div>
</template>

<script setup lang="ts">
/** 证书页编排壳：刷新 / 信任 CA / 检查，数据交给 CertTable。 */
import { computed, ref } from 'vue';
import { ShieldCheck } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
import { useCertsStore } from '../../stores/certs';
import FhButton from '../../components/ui/FhButton.vue';
import PageTitle from '../../components/common/PageTitle.vue';
import TrustDialog from '../../components/common/TrustDialog.vue';
import CertCheckDialog from '../../components/common/CertCheckDialog.vue';
import CertTable from './components/CertTable.vue';

const { t } = useI18n();
const certs = useCertsStore();
const trustOpen = ref(false);
const checkOpen = ref(false);
const checkDomain = ref('');
const query = ref('');

const filteredCerts = computed(() => {
  const q = query.value.trim().toLowerCase();
  if (!q) return certs.items;
  return certs.items.filter((c) =>
    [c.domain, c.subject, c.names, c.file].join(' ').toLowerCase().includes(q));
});

function openCheck(domain: string): void {
  checkDomain.value = domain;
  checkOpen.value = true;
}
// 证书状态随时间变化，30 秒刷新一次；窗口隐藏时自动暂停。
usePolling(() => { void certs.refresh(); }, 30000, computed(() => true));
</script>

<template>
  <PageTitle :title="t('nav.certs')">
    <span class="page-title__actions">
      <FhButton @click="trustOpen = true">
        <ShieldCheck :size="14" /><span>{{ t('certs.trust') }}</span>
      </FhButton>
    </span>
  </PageTitle>

  <div class="main__body">
    <p v-if="certs.error" class="notice notice--err">{{ certs.error }}</p>
    <div v-if="certs.loading && !certs.items.length" class="notice">{{ t('certs.loading') }}</div>

    <input
      v-model="query" class="list-search" type="search"
      :placeholder="t('certs.searchPh')" :aria-label="t('certs.searchPh')"
    />

    <CertTable :items="filteredCerts" :loading="certs.loading" :filtered="query.trim().length > 0" @check="openCheck" />

    <TrustDialog v-model:open="trustOpen" />
    <CertCheckDialog v-model:open="checkOpen" :domain="checkDomain" />
  </div>
</template>

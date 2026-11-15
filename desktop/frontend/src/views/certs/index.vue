<script setup lang="ts">
/** 证书页编排壳：刷新 / 信任 CA / 检查，数据交给 CertTable。 */
import { onMounted, onUnmounted, ref } from 'vue';
import { ShieldCheck } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useCertsStore } from '../../stores/certs';
import FhButton from '../../components/ui/FhButton.vue';
import TrustDialog from '../../components/common/TrustDialog.vue';
import CertCheckDialog from '../../components/common/CertCheckDialog.vue';
import CertTable from './components/CertTable.vue';

const { t } = useI18n();
const certs = useCertsStore();
const trustOpen = ref(false);
const checkOpen = ref(false);
const checkDomain = ref('');

function openCheck(domain: string): void {
  checkDomain.value = domain;
  checkOpen.value = true;
}
onMounted(() => {
  void certs.refresh();
  // 自动刷新：证书状态随时间变化，30 秒一次，无需手动点。
  const timer = window.setInterval(() => { void certs.refresh(); }, 30000);
  onUnmounted(() => window.clearInterval(timer));
});
</script>

<template>
  <div class="main__body">
    <div class="toolbar">
      <FhButton @click="trustOpen = true">
        <ShieldCheck :size="14" /><span>{{ t('certs.trust') }}</span>
      </FhButton>
    </div>
    <p v-if="certs.error" class="notice notice--err">{{ certs.error }}</p>
    <div v-if="certs.loading && !certs.items.length" class="notice">{{ t('certs.loading') }}</div>

    <CertTable :items="certs.items" :loading="certs.loading" @check="openCheck" />

    <TrustDialog v-model:open="trustOpen" />
    <CertCheckDialog v-model:open="checkOpen" :domain="checkDomain" />
  </div>
</template>

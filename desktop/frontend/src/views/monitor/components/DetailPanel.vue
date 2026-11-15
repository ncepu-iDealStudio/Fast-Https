<script setup lang="ts">
/** 运行数据明细：顶部 tab（连接/请求/响应/上游）+ 左右两列键值。 */
import { ref } from 'vue';
import { useI18n } from '../../../composables/useI18n';

type Group = Record<string, string | number>;
const props = defineProps<{
  conns: Group; reqs: Group; resp: Group; upstream: Group;
  title: string; note: string;
}>();
const { t } = useI18n();

const TABS = [
  { key: 'conns', label: 'runtime.cardConn' },
  { key: 'reqs', label: 'runtime.cardReq' },
  { key: 'resp', label: 'runtime.cardResp' },
  { key: 'upstream', label: 'runtime.cardUpstream' },
] as const;
const active = ref<'conns' | 'reqs' | 'resp' | 'upstream'>('conns');

function groupOf(key: typeof active.value): Group {
  return props[key];
}
function rows(g: Group): { k: string; v: string | number }[] {
  return Object.entries(g).map(([k, v]) => ({ k, v }));
}
</script>

<template>
  <section class="panel">
    <div class="panel__head">
      <h3>{{ title }}</h3>
      <small class="panel__note mono">PROCESS METRICS</small>
    </div>
    <div class="detail-tabs">
      <button
        v-for="tab in TABS" :key="tab.key"
        class="detail-tab" :class="{ 'detail-tab--on': active === tab.key }"
        @click="active = tab.key"
      >{{ t(tab.label) }}</button>
    </div>
    <div class="detail-cols">
      <div v-for="r in rows(groupOf(active))" :key="r.k" class="detail-line">
        <span class="mono">{{ r.k }}</span><b class="mono">{{ r.v }}</b>
      </div>
    </div>
    <p class="detail-note">{{ t('runtime.pageNote') }}</p>
  </section>
</template>

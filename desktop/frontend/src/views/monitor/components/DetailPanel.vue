<script setup lang="ts">
/**
 * 运行数据明细：顶部 tab 切换分组，双列键值，底部说明。
 * 每组数据为空时显示占位符，不显示 0 冒充真实读数。
 */
import { computed, ref } from 'vue';
import { useI18n } from '../../../composables/useI18n';

type Group = Record<string, number>;

const props = defineProps<{
  conns: Group;
  reqs: Group;
  resp: Group;
  upstream: Group;
  title: string;
  note: string;
}>();
const { t } = useI18n();

const TABS = [
  { key: 'conns', label: 'runtime.cardConn' },
  { key: 'reqs', label: 'runtime.cardReq' },
  { key: 'resp', label: 'runtime.cardResp' },
  { key: 'upstream', label: 'runtime.cardUpstream' },
] as const;
type TabKey = (typeof TABS)[number]['key'];

const active = ref<TabKey>('conns');

const rows = computed(() => {
  const g = props[active.value] ?? {};
  return Object.entries(g).map(([k, v]) => ({ k, v: Number(v) }));
});
</script>

<template>
  <section class="pane">
    <div class="pane__head">
      <h3>{{ title }}</h3>
      <span class="pane__note">{{ note }}</span>
    </div>
    <div class="tabs">
      <button
        v-for="tab in TABS" :key="tab.key"
        class="tab" :class="{ 'tab--on': active === tab.key }"
        @click="active = tab.key"
      >{{ t(tab.label) }}</button>
    </div>
    <div class="kv2">
      <div v-for="r in rows" :key="r.k" class="kv2__row">
        <span class="kv2__k">{{ r.k }}</span>
        <b class="kv2__v" :class="{ 'is-zero': r.v === 0 }">{{ r.v }}</b>
      </div>
      <div v-if="!rows.length" class="kv2__empty">{{ t('monitor.noData') }}</div>
    </div>
    <div class="pane__foot">
      <span>{{ t('runtime.pageNote') }}</span>
    </div>
  </section>
</template>

<script setup lang="ts">
/** 站点页编排壳：增删改启停，渲染交给 SiteList。 */
import { computed, ref } from 'vue';
import { Plus } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { usePolling } from '../../composables/usePolling';
import { useSitesStore } from '../../stores/sites';
import { useOverviewStore } from '../../stores/overview';
import FhButton from '../../components/ui/FhButton.vue';
import PageTitle from '../../components/common/PageTitle.vue';
import SiteDialog from '../../components/common/SiteDialog.vue';
import SiteList from './components/SiteList.vue';
import ConfirmDialog from '../../components/common/ConfirmDialog.vue';
import { openConfigDir } from '../../services/api/app';
import { messageOf } from '../../services/api/base';

const { t } = useI18n();
const sites = useSitesStore();
const overview = useOverviewStore();
const busyIds = ref<string[]>([]);
function setBusy(id: string, on: boolean): void {
  busyIds.value = on ? [...busyIds.value, id] : busyIds.value.filter((x) => x !== id);
}
const notice = ref('');
const dialogOpen = ref(false);
/** 删除确认：先弹窗再执行，避免误点直接删除 */
const delOpen = ref(false);
const delId = ref('');
const delBusy = ref(false);
const editingId = ref<string | undefined>(undefined);
const query = ref('');

// 站点多起来后按名称 / 端口快速过滤。
const filteredSites = computed(() => {
  const q = query.value.trim().toLowerCase();
  if (!q) return sites.items;
  return sites.items.filter((s) =>
    [s.name, s.listen, ...(s.listens ?? [])].join(' ').toLowerCase().includes(q));
});

/** 服务是否在运行：停止时链接灰化。 */
const serverUp = computed(() => overview.data?.running ?? false);

// 站点列表随配置事务变化，15 秒刷新一次；窗口隐藏时自动暂停。
usePolling(() => { void sites.refresh(); }, 15000, computed(() => true));

function openAdd(): void {
  editingId.value = undefined;
  dialogOpen.value = true;
}
function openEdit(id: string): void {
  editingId.value = id;
  dialogOpen.value = true;
}
async function onSaved(): Promise<void> {
  await sites.refresh();
}

async function onToggle(id: string, enabled: boolean): Promise<void> {
  setBusy(id, true);
  try {
    await sites.toggle(id, enabled);
    notice.value = t(enabled ? 'sites.toggledOn' : 'sites.toggledOff', { name: id });
  } catch (e) {
    notice.value = messageOf(e);
  } finally {
    setBusy(id, false);
  }
}

/** 点删除：只打开确认框，不动数据。 */
function askDelete(id: string): void {
  delId.value = id;
  delOpen.value = true;
}

/** 确认后执行删除。 */
async function doDelete(): Promise<void> {
  const id = delId.value;
  if (!id) return;
  delBusy.value = true;
  setBusy(id, true);
  try {
    await sites.remove(id);
    notice.value = t('sites.deleted', { name: id });
    delOpen.value = false;
    delId.value = '';
  } catch (e) {
    notice.value = messageOf(e);
    delOpen.value = false;
  } finally {
    delBusy.value = false;
    setBusy(id, false);
  }
}

async function onCopy(url: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(url);
    notice.value = t('sites.copied', { url });
  } catch {
    notice.value = url;
  }
}

async function onOpen(url: string): Promise<void> {
  try {
    await openConfigDir(url);
  } catch (e) {
    notice.value = messageOf(e);
  }
}
</script>

<template>
  <PageTitle :title="t('nav.sites')">
    <span class="page-title__actions">
      <FhButton variant="primary" @click="openAdd">
        <Plus :size="14" /><span>{{ t('sites.add') }}</span>
      </FhButton>
    </span>
  </PageTitle>

  <div class="main__body">
    <p v-if="sites.error" class="notice notice--err">{{ sites.error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <div v-if="sites.loading && !sites.items.length" class="notice">{{ t('sites.loading') }}</div>

    <input
      v-model="query" class="list-search" type="search"
      :placeholder="t('sites.searchPh')" :aria-label="t('sites.searchPh')"
    />

    <SiteList
      :items="filteredSites" :loading="sites.loading" :server-up="serverUp" :busy-ids="busyIds"
      :filtered="query.trim().length > 0"
      @edit="openEdit" @toggle="onToggle" @delete="askDelete" @copy="onCopy" @open="onOpen"
    />

    <SiteDialog v-model:open="dialogOpen" :editing-id="editingId" @saved="onSaved" />

    <ConfirmDialog
      v-model:open="delOpen"
      :title="t('sites.delConfirmTitle')"
      :message="t('sites.delConfirmBody', { name: delId })"
      :loading="delBusy"
      @confirm="doDelete"
    />
  </div>
</template>

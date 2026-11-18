<script setup lang="ts">
/** 站点页编排壳：增删改启停，渲染交给 SiteList。 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { Plus } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
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
const busy = ref('');
const notice = ref('');
const dialogOpen = ref(false);
/** 删除确认：先弹窗再执行，避免误点直接删除 */
const delOpen = ref(false);
const delId = ref('');
const delBusy = ref(false);
const editingId = ref<string | undefined>(undefined);

onMounted(() => {
  void sites.refresh();
  // 自动刷新：站点列表随配置事务变化，15 秒一次。
  const timer = window.setInterval(() => { void sites.refresh(); }, 15000);
  onUnmounted(() => window.clearInterval(timer));
});

/** 服务是否在运行：停止时链接灰化。 */
const serverUp = computed(() => overview.data?.running ?? false);

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
  busy.value = id;
  try {
    notice.value = await sites.toggle(id, enabled);
  } catch (e) {
    notice.value = messageOf(e);
  } finally {
    busy.value = '';
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
  busy.value = id;
  try {
    notice.value = await sites.remove(id);
    delOpen.value = false;
    delId.value = '';
  } catch (e) {
    notice.value = messageOf(e);
    delOpen.value = false;
  } finally {
    delBusy.value = false;
    busy.value = '';
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

    <SiteList
      :items="sites.items" :loading="sites.loading" :server-up="serverUp" :busy="busy"
      @edit="openEdit" @toggle="onToggle" @delete="askDelete" @copy="onCopy" @open="onOpen"
    />

    <SiteDialog v-model:open="dialogOpen" :editing-id="editingId" @saved="onSaved" />

    <ConfirmDialog
      v-model:open="delOpen"
      :title="t('sites.delConfirmTitle')"
      :message="t('sites.delConfirmBody')"
      :loading="delBusy"
      @confirm="doDelete"
    />
  </div>
</template>

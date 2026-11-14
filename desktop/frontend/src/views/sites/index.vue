<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Plus, Power, Trash2, RefreshCw, Pencil } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useSitesStore } from '../../stores/sites';
import FhButton from '../../components/ui/FhButton.vue';
import SiteDialog from '../../components/common/SiteDialog.vue';
import { messageOf } from '../../services/api/base';

const { t } = useI18n();
const sites = useSitesStore();
const busy = ref('');
const notice = ref('');
const dialogOpen = ref(false);
const editingId = ref<string | undefined>(undefined);

onMounted(() => { void sites.refresh(); });

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

async function onDelete(id: string): Promise<void> {
  busy.value = id;
  try {
    notice.value = await sites.remove(id);
  } catch (e) {
    notice.value = messageOf(e);
  } finally {
    busy.value = '';
  }
}
</script>

<template>
  <div class="main__body">
    <p class="lede" v-html="t('sites.lede')" />

    <div class="toolbar">
      <FhButton :loading="sites.loading" @click="sites.refresh()">
        <RefreshCw :size="14" /><span>{{ t('common.refresh') }}</span>
      </FhButton>
      <FhButton variant="primary" @click="openAdd">
        <Plus :size="14" /><span>{{ t('sites.add') }}</span>
      </FhButton>
    </div>

    <p v-if="sites.error" class="notice notice--err">{{ sites.error }}</p>
    <p v-else-if="notice" class="notice">{{ notice }}</p>

    <div v-if="sites.loading && !sites.items.length" class="notice">{{ t('sites.loading') }}</div>

    <div class="table">
      <div class="table__head">
        <span>{{ t('sites.configured') }}</span>
        <span class="mono">listen</span>
        <span>{{ t('common.status') }}</span>
        <span class="table__ops" />
      </div>
      <div v-if="!sites.items.length && !sites.loading" class="table__empty">
        {{ t('sites.empty') }}
      </div>
      <div v-for="s in sites.items" :key="s.id" class="table__row">
        <span class="mono ellipsis" :title="s.name">{{ s.name }}</span>
        <span class="mono muted">{{ s.listen }}</span>
        <span>
          <span class="badge" :class="s.enabled ? 'badge--ok' : 'badge--off'">
            {{ s.enabled ? t('sites.stateOn') : t('sites.stateOff') }}
          </span>
        </span>
        <span class="table__ops">
          <button class="icon-btn" :title="t('sites.detail')" @click="openEdit(s.id)">
            <Pencil :size="14" />
          </button>
          <button
            class="icon-btn" :disabled="busy === s.id"
            :title="s.enabled ? t('sites.toggleOn') : t('sites.toggleOff')"
            @click="onToggle(s.id, !s.enabled)"
          >
            <Power :size="14" />
          </button>
          <button
            class="icon-btn icon-btn--danger" :disabled="busy === s.id"
            :title="t('sites.del')" @click="onDelete(s.id)"
          >
            <Trash2 :size="14" />
          </button>
        </span>
      </div>
    </div>

    <SiteDialog v-model:open="dialogOpen" :editing-id="editingId" @saved="onSaved" />
  </div>
</template>

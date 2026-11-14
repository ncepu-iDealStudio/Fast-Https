<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Plus, Power, Trash2, RefreshCw, Pencil, Copy, ExternalLink, Globe, ChevronRight } from '@lucide/vue';
import { useI18n } from '../../composables/useI18n';
import { useSitesStore } from '../../stores/sites';
import { useOverviewStore } from '../../stores/overview';
import FhButton from '../../components/ui/FhButton.vue';
import SiteDialog from '../../components/common/SiteDialog.vue';
import { openConfigDir } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import type { Site } from '../../types/model';

const { t } = useI18n();
const sites = useSitesStore();
const overview = useOverviewStore();
const busy = ref('');
const notice = ref('');
const dialogOpen = ref(false);
const editingId = ref<string | undefined>(undefined);
const expanded = ref<string>('');

onMounted(() => { void sites.refresh(); });

/** 服务是否在运行：停止时链接灰化。 */
const serverUp = computed(() => overview.data?.running ?? false);

function toggleExpand(id: string): void {
  expanded.value = expanded.value === id ? '' : id;
}

/** URL 生成：与原版 siteURL 同逻辑。TLS 站点优先挑带 ssl 的监听。 */
function siteURLs(s: Site): string[] {
  const listens = Array.isArray(s.listens) && s.listens.length ? s.listens : [s.listen || ''];
  const host = s.name || 'localhost';
  const urls: string[] = [];
  const seen = new Set<string>();
  for (const l of listens) {
    const isTLS = /ssl/i.test(l);
    const scheme = isTLS ? 'https' : 'http';
    const port = String(l).trim().split(/\s+/)[0].replace(/^:/, '');
    const isDefault = (scheme === 'https' && port === '443') || (scheme === 'http' && port === '80');
    const url = scheme + '://' + host + (port && !isDefault ? ':' + port : '') + '/';
    if (!seen.has(url)) { seen.add(url); urls.push(url); }
  }
  return urls;
}

async function copyURL(url: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(url);
    notice.value = t('sites.copied', { url });
  } catch {
    notice.value = url;
  }
}

async function openURL(url: string): Promise<void> {
  try {
    await openConfigDir(url);
  } catch (e) {
    notice.value = messageOf(e);
  }
}

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

    <div class="sitelist">
      <template v-for="s in sites.items" :key="s.id">
        <div
          class="sitelist__row" :class="{ 'sitelist__row--open': expanded === s.id }"
          @click="toggleExpand(s.id)"
        >
          <span class="sitelist__chevron" :class="{ 'is-open': expanded === s.id }">
            <ChevronRight :size="14" />
          </span>
          <span class="sitelist__name mono">{{ s.name }}</span>
          <span class="sitelist__listen mono muted">{{ s.listen }}</span>
          <span>
            <span class="badge" :class="s.enabled ? 'badge--ok' : 'badge--off'">
              {{ s.enabled ? t('sites.stateOn') : t('sites.stateOff') }}
            </span>
          </span>
          <span class="sitelist__ops" @click.stop>
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

        <!-- 展开区：只放访问链接，复制 / 打开用图标 -->
        <div v-if="expanded === s.id" class="sitelist__panel">
          <div class="sitelist__panel-label">{{ t('sites.urlsTitle') }}</div>
          <div
            v-for="url in siteURLs(s)" :key="url"
            class="sitelist__url" :class="{ 'sitelist__url--off': !serverUp || !s.enabled }"
          >
            <Globe :size="14" class="sitelist__url-ico" />
            <span class="sitelist__url-text mono">{{ url }}</span>
            <span class="sitelist__url-ops">
              <button class="icon-btn" :title="t('sites.copy')" @click="copyURL(url)">
                <Copy :size="13" />
              </button>
              <button
                class="icon-btn" :title="t('sites.open')"
                :disabled="!serverUp || !s.enabled"
                @click="openURL(url)"
              >
                <ExternalLink :size="13" />
              </button>
            </span>
          </div>
        </div>
      </template>

      <div v-if="!sites.items.length && !sites.loading" class="table__empty">
        {{ t('sites.empty') }}
      </div>
    </div>

    <SiteDialog v-model:open="dialogOpen" :editing-id="editingId" @saved="onSaved" />
  </div>
</template>

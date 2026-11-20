<script setup lang="ts">
/** 站点列表：行点击展开访问链接，行内编辑/启停/删除。 */
import { computed, ref } from 'vue';
import { ChevronRight, Pencil, Power, Trash2, Copy, ExternalLink, Globe } from '@lucide/vue';
import { useI18n } from '../../../composables/useI18n';
import { quipOfDay } from '../../../content/quips';
import type { Site } from '../../../types/model';

const props = defineProps<{
  items: Site[];
  loading: boolean;
  serverUp: boolean;
  busyIds: string[];
  filtered?: boolean;
}>();
const emit = defineEmits<{
  edit: [id: string];
  toggle: [id: string, enabled: boolean];
  delete: [id: string];
  copy: [url: string];
  open: [url: string];
}>();
const { t, lang } = useI18n();
const quip = computed(() => quipOfDay(lang()));
const expanded = ref('');

function toggleExpand(id: string): void {
  expanded.value = expanded.value === id ? '' : id;
}

/** URL 生成沿用原版 siteURL：TLS 优先挑带 ssl 的监听，非默认端口显式带上。 */
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
</script>

<template>
  <div class="sitelist">
    <template v-for="s in items" :key="s.id">
      <div
        class="sitelist__row" :class="{ 'sitelist__row--open': expanded === s.id }"
        role="button" tabindex="0" :aria-expanded="expanded === s.id"
        @click="toggleExpand(s.id)"
        @keydown.enter.prevent="toggleExpand(s.id)"
        @keydown.space.prevent="toggleExpand(s.id)"
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
          <button class="icon-btn" :title="t('sites.detail')" :aria-label="t('sites.detail')" @click="emit('edit', s.id)">
            <Pencil :size="14" />
          </button>
          <button
            class="icon-btn" :disabled="busyIds.includes(s.id)"
            :title="s.enabled ? t('sites.toggleOn') : t('sites.toggleOff')"
            :aria-label="s.enabled ? t('sites.toggleOn') : t('sites.toggleOff')"
            @click="emit('toggle', s.id, !s.enabled)"
          >
            <Power :size="14" />
          </button>
          <button
            class="icon-btn icon-btn--danger" :disabled="busyIds.includes(s.id)"
            :title="t('sites.del')" :aria-label="t('sites.del')" @click="emit('delete', s.id)"
          >
            <Trash2 :size="14" />
          </button>
        </span>
      </div>

      <div v-if="expanded === s.id" class="sitelist__panel">
        <div class="sitelist__panel-label">{{ t('sites.urlsTitle') }}</div>
        <div
          v-for="url in siteURLs(s)" :key="url"
          class="sitelist__url" :class="{ 'sitelist__url--off': !serverUp || !s.enabled }"
        >
          <Globe :size="14" class="sitelist__url-ico" />
          <span class="sitelist__url-text mono">{{ url }}</span>
          <span class="sitelist__url-ops">
            <button class="icon-btn" :title="t('sites.copy')" :aria-label="t('sites.copy')" @click="emit('copy', url)">
              <Copy :size="13" />
            </button>
            <button
              class="icon-btn" :title="t('sites.open')" :aria-label="t('sites.open')"
              :disabled="!serverUp || !s.enabled"
              @click="emit('open', url)"
            >
              <ExternalLink :size="13" />
            </button>
          </span>
        </div>
      </div>
    </template>

    <div v-if="!items.length && !loading" class="table__empty">
      {{ filtered ? t('sites.noMatch') : t('sites.empty') }}
      <span v-if="!filtered" class="table__quip">{{ quip }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { addSite, readSiteDetail, saveSiteDetail } from '../../services/api/site';
import { pickPath } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import type { SiteInput } from '../../services/api/types';

const props = defineProps<{ open: boolean; editingId?: string }>();
const emit = defineEmits<{ 'update:open': [v: boolean]; saved: [] }>();
const { t } = useI18n();

const editing = computed(() => !!props.editingId);
const busy = ref(false);
const error = ref('');
const info = ref<{ k: string; v: string }[]>([]);
const raw = ref('');

const form = reactive({
  name: '',
  proto: 'http' as 'http' | 'https' | 'both',
  httpPort: '80',
  httpsPort: '443',
  listen: '80',
  type: 'local' as 'local' | 'proxy',
  root: '',
  proxyScheme: 'http://',
  proxy: '',
  zipGzip: false,
  zipBr: false,
  limitRate: 0,
  limitBurst: 0,
  limitMem: 0,
  limitNodelay: false,
  authOn: false,
  authUser: '',
  authPass: '',
  authRealm: '',
  cachePath: '',
  cacheValid: '',
  cacheMaxSize: 0,
});

/** 协议 + 端口 → listen 串，可再手动微调（如加 h2）。 */
function syncListen(): void {
  if (form.proto === 'http') form.listen = form.httpPort;
  else if (form.proto === 'https') form.listen = `${form.httpsPort} ssl h2`;
  else form.listen = `${form.httpPort}, ${form.httpsPort} ssl h2`;
}
watch(() => [form.proto, form.httpPort, form.httpsPort], syncListen);

watch(() => props.open, async (open) => {
  if (!open) return;
  error.value = '';
  info.value = [];
  raw.value = '';
  if (!props.editingId) {
    Object.assign(form, {
      name: '', proto: 'http', httpPort: '80', httpsPort: '443', listen: '80',
      type: 'local', root: '', proxyScheme: 'http://', proxy: '',
      zipGzip: false, zipBr: false, limitRate: 0, limitBurst: 0, limitMem: 0,
      limitNodelay: false, authOn: false, authUser: '', authPass: '', authRealm: '',
      cachePath: '', cacheValid: '', cacheMaxSize: 0,
    });
    return;
  }
  busy.value = true;
  try {
    const d = await readSiteDetail(props.editingId);
    form.name = d.name;
    form.listen = d.listen;
    form.type = d.type === 'proxy' ? 'proxy' : 'local';
    form.root = d.root ?? '';
    const mp = /^(https?:\/\/)?(.*)$/.exec(d.proxyPass ?? '');
    form.proxyScheme = mp?.[1] || 'http://';
    form.proxy = mp?.[2] ?? '';
    form.zipGzip = (d.zip ?? []).includes('gzip');
    form.zipBr = (d.zip ?? []).some((z) => z === 'br' || z === 'brotli');
    form.limitRate = d.limitRate ?? 0;
    form.limitBurst = d.limitBurst ?? 0;
    form.limitMem = d.limitMem ?? 0;
    form.limitNodelay = !!d.limitNodelay;
    form.authOn = !!d.authOn;
    form.authUser = d.authUser ?? '';
    form.authPass = '';
    form.authRealm = d.authRealm ?? '';
    form.cachePath = d.cachePath ?? '';
    form.cacheValid = (d.cacheValid ?? []).join(', ');
    form.cacheMaxSize = d.cacheMaxSize ?? 0;
    info.value = [
      { k: t('dialog.site.cert'), v: d.cert ?? '' },
      { k: t('dialog.site.certKey'), v: d.certKey ?? '' },
      { k: t('dialog.site.index'), v: d.index ?? '' },
      { k: t('dialog.site.path'), v: d.path ?? '' },
    ].filter((r) => r.v);
    raw.value = d.raw ?? '';
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    busy.value = false;
  }
}, { immediate: true });

function buildPayload(): SiteInput {
  const zip: string[] = [];
  if (form.zipGzip) zip.push('gzip');
  if (form.zipBr) zip.push('br');
  return {
    id: props.editingId ?? '',
    name: form.name,
    listen: form.listen,
    type: form.type,
    root: form.type === 'local' ? form.root : '',
    proxyPass: form.type === 'proxy' ? form.proxyScheme + form.proxy : '',
    zip,
    limitRate: form.limitRate,
    limitBurst: form.limitBurst,
    limitMem: form.limitMem,
    limitNodelay: form.limitNodelay,
    authOn: form.authOn,
    authUser: form.authOn ? form.authUser : '',
    authPass: form.authOn ? form.authPass : '',
    authRealm: form.authOn ? form.authRealm : '',
    cachePath: form.cachePath.trim(),
    cacheValid: form.cacheValid.split(',').map((s) => s.trim()).filter(Boolean),
    cacheMaxSize: form.cacheMaxSize,
  };
}

async function submit(): Promise<void> {
  busy.value = true;
  error.value = '';
  try {
    if (editing.value) await saveSiteDetail(buildPayload());
    else await addSite(buildPayload());
    emit('saved');
    emit('update:open', false);
  } catch (e) {
    error.value = messageOf(e);
  } finally {
    busy.value = false;
  }
}

async function browse(): Promise<void> {
  try {
    const p = await pickPath('dir');
    if (p) form.root = p;
  } catch (e) {
    error.value = messageOf(e);
  }
}
</script>

<template>
  <FhDialog
    :open="open" :title="editing ? t('dialog.site.titleDetail') : t('dialog.site.titleAdd')"
    width="620px" @update:open="(v: boolean) => emit('update:open', v)"
  >
    <!-- ── 基本信息：域名 / 类型 / 根目录 ── -->
    <section class="fset">
      <div class="fset__row">
        <label class="fset__label">
          <span>{{ t('dialog.site.name') }}</span>
          <small>{{ t('dialog.site.nameHint') }}</small>
        </label>
        <div class="fset__ctrl">
          <input v-model="form.name" type="text" placeholder="example.com" class="wide" />
        </div>
      </div>

      <div class="fset__row">
        <label class="fset__label"><span>{{ t('dialog.site.type') }}</span></label>
        <div class="fset__ctrl">
          <select v-model="form.type" class="mid">
            <option value="local">{{ t('dialog.site.typeLocal') }}</option>
            <option value="proxy">{{ t('dialog.site.typeProxy') }}</option>
          </select>
        </div>
      </div>

      <div v-if="form.type === 'local'" class="fset__row">
        <label class="fset__label">
          <span>{{ t('dialog.site.root') }}</span>
          <small>{{ t('dialog.site.rootHint') }}</small>
        </label>
        <div class="fset__ctrl">
          <div class="joined">
            <input v-model="form.root" type="text" placeholder="./httpdoc/root" />
            <FhButton size="small" @click="browse">{{ t('dialog.site.browse') }}</FhButton>
          </div>
        </div>
      </div>

      <div v-else class="fset__row">
        <label class="fset__label">
          <span>{{ t('dialog.site.proxy') }}</span>
          <small>{{ t('dialog.site.proxyNote') }}</small>
        </label>
        <div class="fset__ctrl">
          <div class="joined">
            <select v-model="form.proxyScheme" class="scheme">
              <option value="http://">http://</option>
              <option value="https://">https://</option>
            </select>
            <input v-model="form.proxy" type="text" placeholder="127.0.0.1:8080" />
          </div>
        </div>
      </div>
    </section>

    <!-- ── 监听：协议 + 端口一行，listen 单独一行可微调 ── -->
    <section class="fset">
      <div class="fset__head">{{ t('dialog.site.proto') }}</div>
      <div class="fset__row">
        <label class="fset__label"><span>{{ t('dialog.site.proto') }}</span></label>
        <div class="fset__ctrl">
          <div class="joined">
            <select v-model="form.proto" class="mid">
              <option value="http">{{ t('dialog.site.protoHttp') }}</option>
              <option value="https">{{ t('dialog.site.protoHttps') }}</option>
              <option value="both">{{ t('dialog.site.protoBoth') }}</option>
            </select>
            <label v-if="form.proto !== 'https'" class="mini-field">
              <span>HTTP</span><input v-model="form.httpPort" type="number" class="port" />
            </label>
            <label v-if="form.proto !== 'http'" class="mini-field">
              <span>HTTPS</span><input v-model="form.httpsPort" type="number" class="port" />
            </label>
          </div>
        </div>
      </div>
      <div class="fset__row">
        <label class="fset__label"><span class="mono">listen</span></label>
        <div class="fset__ctrl">
          <input v-model="form.listen" type="text" class="mono wide" />
          <small class="field-note">{{ t('dialog.site.listenNote') }}</small>
        </div>
      </div>
    </section>

    <!-- ── 高级配置 ── -->
    <details class="adv">
      <summary>{{ t('dialog.site.advanced') }}</summary>
      <p class="field-note">{{ t('dialog.site.advancedHint') }}</p>

      <div class="adv__grid">
        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.zip') }}</div>
          <div class="choice-group">
            <label class="mini-field"><input v-model="form.zipGzip" type="checkbox" /><span>gzip</span></label>
            <label class="mini-field"><input v-model="form.zipBr" type="checkbox" /><span>brotli</span></label>
          </div>
          <small class="field-note">{{ t('dialog.site.zipHint') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.limit') }}</div>
          <div class="grid3">
            <label class="mini-field"><span>{{ t('dialog.site.limitRate') }}</span><input v-model.number="form.limitRate" type="number" min="0" /></label>
            <label class="mini-field"><span>{{ t('dialog.site.limitBurst') }}</span><input v-model.number="form.limitBurst" type="number" min="0" /></label>
            <label class="mini-field"><span>{{ t('dialog.site.limitMem') }}</span><input v-model.number="form.limitMem" type="number" min="0" /></label>
          </div>
          <label class="mini-field"><input v-model="form.limitNodelay" type="checkbox" /><span>{{ t('dialog.site.limitNodelay') }}</span></label>
          <small class="field-note">{{ t('dialog.site.limitHint') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.auth') }}</div>
          <label class="mini-field"><input v-model="form.authOn" type="checkbox" /><span>{{ t('dialog.site.authOn') }}</span></label>
          <div v-if="form.authOn" class="grid2">
            <label class="mini-field"><span>{{ t('dialog.site.authUser') }}</span><input v-model="form.authUser" type="text" autocomplete="off" /></label>
            <label class="mini-field"><span>{{ t('dialog.site.authPass') }}</span><input v-model="form.authPass" type="password" autocomplete="new-password" /></label>
            <label class="mini-field"><span>{{ t('dialog.site.authRealm') }}</span><input v-model="form.authRealm" type="text" /></label>
          </div>
          <small class="field-note">{{ t('dialog.site.authHint') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.cache') }}</div>
          <div class="joined">
            <input v-model="form.cachePath" type="text" placeholder="./cache" />
          </div>
          <div class="grid2">
            <label class="mini-field"><span>{{ t('dialog.site.cacheValid') }}</span><input v-model="form.cacheValid" type="text" placeholder="10s, 1m" /></label>
            <label class="mini-field"><span>{{ t('dialog.site.cacheMaxSize') }}</span><input v-model.number="form.cacheMaxSize" type="number" min="0" /></label>
          </div>
          <small class="field-note">{{ t('dialog.site.cacheNote') }}</small>
        </div>
      </div>
    </details>

    <!-- ── 只读信息 ── -->
    <details v-if="editing && info.length" class="adv">
      <summary>{{ t('dialog.site.readonly') }}</summary>
      <div class="kv-table">
        <div v-for="r in info" :key="r.k" class="kv-row">
          <span class="k">{{ r.k }}</span><span class="v mono">{{ r.v }}</span>
        </div>
      </div>
      <details v-if="raw" class="adv">
        <summary>{{ t('dialog.site.raw') }}</summary>
        <pre class="cmd-result">{{ raw }}</pre>
      </details>
    </details>

    <p v-if="error" class="dialog-error">{{ error }}</p>

    <template #footer>
      <FhButton @click="emit('update:open', false)">{{ t('common.cancel') }}</FhButton>
      <FhButton variant="primary" :loading="busy" @click="submit">
        {{ editing ? t('common.save') : t('dialog.site.titleAdd') }}
      </FhButton>
    </template>
  </FhDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import FhField from '../ui/FhField.vue';
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
const info = ref<Record<string, string>>({});
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

/** 编辑模式回填；添加模式重置为默认值。 */
watch(() => props.open, async (open) => {
  if (!open) return;
  error.value = '';
  info.value = {};
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
    // proxyPass 形如 http://host:port，拆成协议 + 主体
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
    info.value = {
      [t('dialog.site.cert')]: d.cert ?? '',
      [t('dialog.site.certKey')]: d.certKey ?? '',
      [t('dialog.site.index')]: d.index ?? '',
      [t('dialog.site.source')]: d.source ?? '',
    };
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
    width="560px" @update:open="(v: boolean) => emit('update:open', v)"
  >
    <p v-if="editing && info[t('dialog.site.source')]" class="dlg-source mono">
      {{ info[t('dialog.site.source')] }}
    </p>

    <FhField :label="t('dialog.site.name')" :hint="t('dialog.site.nameHint')">
      <input v-model="form.name" type="text" placeholder="example.com" />
    </FhField>

    <FhField :label="t('dialog.site.proto')" :hint="t('dialog.site.protoHint')">
      <div class="inline-fields">
        <select v-model="form.proto">
          <option value="http">{{ t('dialog.site.protoHttp') }}</option>
          <option value="https">{{ t('dialog.site.protoHttps') }}</option>
          <option value="both">{{ t('dialog.site.protoBoth') }}</option>
        </select>
        <label v-if="form.proto !== 'https'" class="mini-field">
          <span>HTTP</span><input v-model="form.httpPort" type="number" />
        </label>
        <label v-if="form.proto !== 'http'" class="mini-field">
          <span>HTTPS</span><input v-model="form.httpsPort" type="number" />
        </label>
      </div>
      <input v-model="form.listen" type="text" class="mono" />
      <small class="field-note">{{ t('dialog.site.listenNote') }}</small>
    </FhField>

    <FhField :label="t('dialog.site.type')">
      <select v-model="form.type">
        <option value="local">{{ t('dialog.site.typeLocal') }}</option>
        <option value="proxy">{{ t('dialog.site.typeProxy') }}</option>
      </select>
    </FhField>

    <FhField v-if="form.type === 'local'" :label="t('dialog.site.root')" :hint="t('dialog.site.rootHint')">
      <div class="inline-fields">
        <input v-model="form.root" type="text" placeholder="./httpdoc/root" />
        <FhButton size="small" @click="browse">{{ t('dialog.site.browse') }}</FhButton>
      </div>
    </FhField>

    <FhField v-else :label="t('dialog.site.proxy')" :hint="t('dialog.site.proxyNote')">
      <div class="inline-fields">
        <select v-model="form.proxyScheme" style="max-width: 110px">
          <option value="http://">http://</option>
          <option value="https://">https://</option>
        </select>
        <input v-model="form.proxy" type="text" placeholder="127.0.0.1:8080" />
      </div>
    </FhField>

    <details class="raw-details">
      <summary>{{ t('dialog.site.advanced') }}</summary>
      <p class="field-note">{{ t('dialog.site.advancedHint') }}</p>

      <FhField :label="t('dialog.site.zip')" :hint="t('dialog.site.zipHint')">
        <div class="choice-group">
          <label class="mini-field"><input v-model="form.zipGzip" type="checkbox" /><span>gzip</span></label>
          <label class="mini-field"><input v-model="form.zipBr" type="checkbox" /><span>brotli（br）</span></label>
        </div>
      </FhField>

      <FhField :label="t('dialog.site.limit')" :hint="t('dialog.site.limitHint')">
        <div class="inline-fields">
          <label class="mini-field"><span>{{ t('dialog.site.limitRate') }}</span><input v-model.number="form.limitRate" type="number" min="0" /></label>
          <label class="mini-field"><span>{{ t('dialog.site.limitBurst') }}</span><input v-model.number="form.limitBurst" type="number" min="0" /></label>
          <label class="mini-field"><span>{{ t('dialog.site.limitMem') }}</span><input v-model.number="form.limitMem" type="number" min="0" /></label>
        </div>
        <label class="mini-field">
          <input v-model="form.limitNodelay" type="checkbox" /><span>{{ t('dialog.site.limitNodelay') }}</span>
        </label>
      </FhField>

      <FhField :label="t('dialog.site.auth')" :hint="t('dialog.site.authHint')">
        <label class="mini-field"><input v-model="form.authOn" type="checkbox" /><span>{{ t('dialog.site.authOn') }}</span></label>
        <div v-if="form.authOn" class="inline-fields">
          <label class="mini-field"><span>{{ t('dialog.site.authUser') }}</span><input v-model="form.authUser" type="text" autocomplete="off" /></label>
          <label class="mini-field"><span>{{ t('dialog.site.authPass') }}</span><input v-model="form.authPass" type="password" autocomplete="new-password" /></label>
          <label class="mini-field"><span>{{ t('dialog.site.authRealm') }}</span><input v-model="form.authRealm" type="text" /></label>
        </div>
      </FhField>

      <FhField :label="t('dialog.site.cache')" :hint="t('dialog.site.cacheHint')">
        <div class="inline-fields">
          <input v-model="form.cachePath" type="text" placeholder="./cache" />
        </div>
        <div class="inline-fields">
          <label class="mini-field"><span>{{ t('dialog.site.cacheValid') }}</span><input v-model="form.cacheValid" type="text" placeholder="10s, 1m" /></label>
          <label class="mini-field"><span>{{ t('dialog.site.cacheMaxSize') }}</span><input v-model.number="form.cacheMaxSize" type="number" min="0" /></label>
        </div>
        <small class="field-note">{{ t('dialog.site.cacheNote') }}</small>
      </FhField>
    </details>

    <details v-if="editing && Object.keys(info).length" class="raw-details">
      <summary>{{ t('dialog.site.readonly') }}</summary>
      <div class="kv-table">
        <div v-for="(v, k) in info" :key="k" class="kv-row">
          <span class="k">{{ k }}</span><span class="v mono">{{ v || '—' }}</span>
        </div>
      </div>
      <details v-if="raw" class="raw-details">
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

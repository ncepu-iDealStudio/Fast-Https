<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from '../../composables/useI18n';
import FhDialog from '../ui/FhDialog.vue';
import FhButton from '../ui/FhButton.vue';
import { addSite, readSiteDetail, saveSiteDetail } from '../../services/api/site';
import { listCacheZones } from '../../services/api/cache';
import { pickAbsPath } from '../../services/api/app';
import { messageOf } from '../../services/api/base';
import type { SiteInput, CacheZone } from '../../services/api/types';

const props = defineProps<{ open: boolean; editingId?: string }>();
const emit = defineEmits<{ 'update:open': [v: boolean]; saved: [] }>();
const { t } = useI18n();

const editing = computed(() => !!props.editingId);
const busy = ref(false);
const error = ref('');
const info = ref<{ k: string; v: string }[]>([]);
const raw = ref('');

const form = reactive({
  // 本机建站基本都从 localhost 起步，默认填好省一次输入；多域名仍可继续写。
  name: 'localhost',
  // 监听：HTTPS 只决定 ssl，HTTP/2 是独立开关。
  https: false,
  h2: true,
  keepHttp: false,
  httpPort: '80',
  httpsPort: '443',
  listen: '80',
  type: 'local' as 'local' | 'proxy',
  root: '',
  proxyScheme: 'http://',
  proxy: '',
  symlinkPolicy: '',
  cert: '',
  certKey: '',
  zipGzip: true,
  zipBr: true,
  zipMinLength: 1024,
  zipSkipTypes: 'image/*, video/*, audio/*, font/woff2, application/zip, application/gzip',
  limitRate: 0,
  limitBurst: 0,
  limitMem: 0,
  limitNodelay: false,
  authOn: false,
  authUser: '',
  authPass: '',
  authRealm: '',
  // 缓存只选缓存区名；目录、容量与有效期都在主配置的 cache 段里。
  cacheZone: '',
});

// 主配置里可用的缓存区，用于下拉选择。
const zones = ref<CacheZone[]>([]);

async function loadZones(): Promise<void> {
  try {
    zones.value = await listCacheZones();
  } catch {
    // 读不到缓存区不该挡住建站/改站：下拉留空，提示去设置页配置。
    zones.value = [];
  }
}

/**
 * 端口输入框是 type="number"，Vue 的 v-model 会自动把值转成 number，
 * 所以这里必须显式收回字符串，否则 listen 会变成数字，提交时后端
 * （Go 的 SiteInput.listen 是 string）解析直接失败。
 */
function portText(v: string | number): string {
  return String(v ?? '').trim();
}

/** 协议开关 + 端口 → listen 串，可再手动微调（如给 HTTPS 补 h2）。 */
function syncListen(): void {
  const http = portText(form.httpPort) || '80';
  const https = portText(form.httpsPort) || '443';
  // ssl 由 HTTPS 决定，h2 是独立开关且只在 TLS 下有意义。
  const suffix = form.https ? ' ssl' + (form.h2 ? ' h2' : '') : '';
  if (!form.https) form.listen = http;
  else if (form.keepHttp) form.listen = `${http}, ${https}${suffix}`;
  else form.listen = `${https}${suffix}`;
}

/** listen 里出现 ssl 才算 TLS：证书区、提交时的证书字段都以它为准。 */
const tlsOn = computed(() => /ssl/i.test(form.listen));

/** listen 一律以字符串提交；用户清空时回落到「协议 + 端口」重新生成。 */
function listenText(): string {
  const cur = portText(form.listen);
  if (cur) return cur;
  syncListen();
  return portText(form.listen);
}

/** number 输入框被清空时 v-model.number 会留下空串，统一收敛成整数。 */
function toInt(v: number | string): number {
  const n = Number(v);
  return Number.isFinite(n) ? Math.trunc(n) : 0;
}

/** 从一条 listen 片段取端口：兼容 ":80" / "127.0.0.1:8080" / "443 ssl h2"。 */
function portOfSegment(seg: string): string {
  const first = seg.trim().split(/\s+/)[0] ?? '';
  const m = /(\d+)$/.exec(first);
  return m ? m[1] : '';
}

/** 从 listen 串反解 HTTP / HTTPS 端口，编辑时回填端口输入框。 */
function portsOf(listen: string): { http: string; https: string } {
  let http = '';
  let https = '';
  for (const seg of String(listen ?? '').split(',')) {
    const port = portOfSegment(seg);
    if (!port) continue;
    if (/ssl/i.test(seg)) {
      if (!https) https = port;
    } else if (!http) {
      http = port;
    }
  }
  return { http, https };
}

/**
 * 详情回填期间挂起 listen 重算，否则「写端口 → watch → syncListen」
 * 会把后端给的原始监听串（如 "443 ssl"）改写成模板串。
 */
let hydrating = false;

watch(
  () => [form.https, form.h2, form.keepHttp, form.httpPort, form.httpsPort],
  () => { if (!hydrating) syncListen(); },
  { flush: 'sync' },
);

// 关掉 HTTPS 时「同时保留 HTTP」不再有意义，顺手复位，避免下次勾选 HTTPS
// 时莫名变成双端口监听。
watch(() => form.https, (on) => { if (!on) form.keepHttp = false; });

watch(() => props.open, async (open) => {
  if (!open) return;
  error.value = '';
  info.value = [];
  raw.value = '';
  void loadZones();
  if (!props.editingId) {
    Object.assign(form, {
      name: 'localhost', https: false, h2: true, keepHttp: false, httpPort: '80', httpsPort: '443', listen: '80',
      type: 'local', root: '', proxyScheme: 'http://', proxy: '', cert: '', certKey: '',
      zipGzip: true, zipBr: true, zipMinLength: 1024,
      zipSkipTypes: 'image/*, video/*, audio/*, font/woff2, application/zip, application/gzip',
      limitRate: 0, limitBurst: 0, limitMem: 0,
      limitNodelay: false, authOn: false, authUser: '', authPass: '', authRealm: '',
      cacheZone: '',
    });
    return;
  }
  busy.value = true;
  try {
    const d = await readSiteDetail(props.editingId);
    const ports = portsOf(d.listen);
    const tls = /ssl/i.test(d.listen ?? '');
    hydrating = true;
    try {
      form.name = d.name;
      form.https = tls;
      form.h2 = /\bh2\b/i.test(d.listen ?? '');
      // 同时配了非 ssl 端口（如 "80, 443 ssl h2"）才勾「同时保留 HTTP」。
      form.keepHttp = tls && !!ports.http;
      form.httpPort = ports.http || '80';
      form.httpsPort = ports.https || '443';
      form.listen = d.listen;
    } finally {
      hydrating = false;
    }
    form.type = d.type === 'proxy' ? 'proxy' : 'local';
    form.root = d.root ?? '';
    const mp = /^(https?:\/\/)?(.*)$/.exec(d.proxyPass ?? '');
    form.proxyScheme = mp?.[1] || 'http://';
    form.proxy = mp?.[2] ?? '';
    form.symlinkPolicy = d.symlinkPolicy ?? '';
    form.cert = d.cert ?? '';
    form.certKey = d.certKey ?? '';
    form.zipGzip = (d.zip ?? []).includes('gzip');
    form.zipBr = (d.zip ?? []).some((z) => z === 'br' || z === 'brotli');
    form.zipMinLength = d.zipMinLength ?? 0;
    form.zipSkipTypes = (d.zipSkipTypes ?? []).join(', ');
    form.limitRate = d.limitRate ?? 0;
    form.limitBurst = d.limitBurst ?? 0;
    form.limitMem = d.limitMem ?? 0;
    form.limitNodelay = !!d.limitNodelay;
    form.authOn = !!d.authOn;
    form.authUser = d.authUser ?? '';
    form.authPass = '';
    form.authRealm = d.authRealm ?? '';
    form.cacheZone = d.cacheZone ?? '';
    info.value = [
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
  // 证书只在有 TLS 监听时有效：切回纯 HTTP 时一并清空，否则 CLI 会拒绝
  // 「有证书却没有 ssl 监听」的配置。
  const tls = /ssl/i.test(listenText());
  return {
    id: props.editingId ?? '',
    name: form.name,
    listen: listenText(),
    type: form.type,
    root: form.type === 'local' ? form.root : '',
    proxyPass: form.type === 'proxy' ? form.proxyScheme + form.proxy : '',
    symlinkPolicy: form.type === 'local' ? form.symlinkPolicy : '',
    cert: tls ? form.cert.trim() : '',
    certKey: tls ? form.certKey.trim() : '',
    zip,
    zipMinLength: toInt(form.zipMinLength),
    zipSkipTypes: form.zipSkipTypes.split(',').map((s) => s.trim()).filter(Boolean),
    limitRate: toInt(form.limitRate),
    limitBurst: toInt(form.limitBurst),
    limitMem: toInt(form.limitMem),
    limitNodelay: form.limitNodelay,
    authOn: form.authOn,
    authUser: form.authOn ? form.authUser : '',
    authPass: form.authOn ? form.authPass : '',
    authRealm: form.authOn ? form.authRealm : '',
    cacheZone: form.cacheZone,
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

/** 用户取消文件选择框不是错误，别把 "cancelled by user" 当红字报出来。 */
function isCancel(e: unknown): boolean {
  const msg = String((e as { message?: string })?.message ?? e ?? '');
  return /cancel/i.test(msg);
}

/** 把路径统一成 Windows 绝对路径写法：去引号、正斜杠转反斜杠、去掉末尾分隔符。 */
function normalizePath(v: string): string {
  const trimmed = v.trim().replace(/^["']+|["']+$/g, '').trim();
  const out = trimmed.replace(/\//g, '\\');
  // 盘根（D:\ 或 D:）保留原样，其它路径去掉末尾多余的 \ 或 \\
  if (/^[A-Za-z]:(\\?)$/.test(out)) return out;
  return out.replace(/\\+$/, '');
}

async function browse(): Promise<void> {
  try {
    const p = await pickAbsPath('dir');
    // 选择器返回的已经是绝对路径，这里再统一一次分隔符与引号，
    // 保证填进输入框的就是最终要用的那个字符串。
    if (p) form.root = normalizePath(p);
  } catch (e) {
    if (!isCancel(e)) error.value = messageOf(e);
  }
}

/**
 * 根目录统一成 Windows 绝对路径写法：去掉粘贴带上的引号、正斜杠转反斜杠。
 * 相对路径一律不做「按当前目录展开」的猜测 —— 谁也说不清是相对哪个目录，
 * 后端会直接拒绝并提示填绝对路径。
 */
function normalizeRoot(): void {
  if (form.root.trim()) form.root = normalizePath(form.root);
}

/** 证书/私钥走文件选择器；留空则由首次启动按域名自动生成。 */
async function browseCert(kind: 'cert' | 'certKey'): Promise<void> {
  try {
    const p = await pickAbsPath('file');
    if (!p) return;
    const value = normalizePath(p);
    if (kind === 'cert') form.cert = value;
    else form.certKey = value;
  } catch (e) {
    if (!isCancel(e)) error.value = messageOf(e);
  }
}

/** 默认证书路径提示：./cert/<主域名>.pem（与 CLI 的约定一致）。 */
const primaryName = computed(() => form.name.trim().split(/\s+/)[0] || '<域名>');
const certPlaceholder = computed(() => `./cert/${primaryName.value}.pem`);
const certKeyPlaceholder = computed(() => `./cert/${primaryName.value}-key.pem`);
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
            <input v-model="form.root" type="text" placeholder="D:\www\site" @blur="normalizeRoot" />
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

    <!-- ── 监听：两个开关定协议，端口一行，listen 可手动微调 ── -->
    <section class="fset">
      <div class="fset__head">{{ t('dialog.site.listenSection') }}</div>

      <div class="fset__row">
        <label class="fset__label"><span>{{ t('dialog.site.proto') }}</span></label>
        <div class="fset__ctrl">
          <div class="check-row">
            <label class="check-field">
              <input v-model="form.https" type="checkbox" />
              <span>{{ t('dialog.site.httpsOn') }}</span>
            </label>
            <label class="check-field" :class="{ 'is-off': !form.https }" :title="t('dialog.site.h2Hint')">
              <input v-model="form.h2" type="checkbox" :disabled="!form.https" />
              <span>{{ t('dialog.site.h2On') }}</span>
            </label>
            <label class="check-field" :class="{ 'is-off': !form.https }">
              <input v-model="form.keepHttp" type="checkbox" :disabled="!form.https" />
              <span>{{ t('dialog.site.keepHttp') }}</span>
            </label>
          </div>
        </div>
      </div>

      <div class="fset__row">
        <label class="fset__label"><span>{{ t('dialog.site.port') }}</span></label>
        <div class="fset__ctrl">
          <div class="port-row">
            <label v-if="!form.https || form.keepHttp" class="mini-field">
              <span>HTTP</span><input v-model="form.httpPort" type="number" class="port" />
            </label>
            <label v-if="form.https" class="mini-field">
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

    <!-- ── 证书：只有 TLS 站点才需要；留空交给首次启动生成 ── -->
    <section v-if="tlsOn" class="fset">
      <div class="fset__head">{{ t('dialog.site.certSection') }}</div>
      <div class="fset__row">
        <label class="fset__label">
          <span>{{ t('dialog.site.cert') }}</span>
          <small>{{ t('dialog.site.certHint') }}</small>
        </label>
        <div class="fset__ctrl">
          <div class="joined">
            <input v-model="form.cert" type="text" class="mono" :placeholder="certPlaceholder" />
            <FhButton size="small" @click="browseCert('cert')">{{ t('dialog.site.browse') }}</FhButton>
          </div>
        </div>
      </div>
      <div class="fset__row">
        <label class="fset__label"><span>{{ t('dialog.site.certKey') }}</span></label>
        <div class="fset__ctrl">
          <div class="joined">
            <input v-model="form.certKey" type="text" class="mono" :placeholder="certKeyPlaceholder" />
            <FhButton size="small" @click="browseCert('certKey')">{{ t('dialog.site.browse') }}</FhButton>
          </div>
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
          <div class="check-row">
            <label class="check-field"><input v-model="form.zipGzip" type="checkbox" /><span>gzip</span></label>
            <label class="check-field"><input v-model="form.zipBr" type="checkbox" /><span>brotli</span></label>
          </div>
          <small class="field-note">{{ t('dialog.site.zipHint') }}</small>
          <div class="adv__fields">
            <label class="stack-field">
              <span>{{ t('dialog.site.zipMinLength') }}</span>
              <input v-model.number="form.zipMinLength" type="number" min="0" />
            </label>
            <label class="stack-field">
              <span>{{ t('dialog.site.zipSkipTypes') }}</span>
              <input v-model="form.zipSkipTypes" type="text" :placeholder="t('dialog.site.zipSkipTypesHint')" />
            </label>
          </div>
          <small class="field-note">{{ t('dialog.site.zipSkipTypesNote') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.symlink') }}</div>
          <label class="stack-field">
            <select v-model="form.symlinkPolicy">
              <option value="">{{ t('dialog.site.symlinkDefault') }}</option>
              <option value="check-at-load">check-at-load</option>
              <option value="reject">reject</option>
              <option value="allow">allow</option>
            </select>
          </label>
          <small class="field-note">{{ t('dialog.site.symlinkHint') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.limit') }}</div>
          <div class="adv__fields adv__fields--3">
            <label class="stack-field">
              <span>{{ t('dialog.site.limitRate') }}</span>
              <input v-model.number="form.limitRate" type="number" min="0" />
            </label>
            <label class="stack-field">
              <span>{{ t('dialog.site.limitBurst') }}</span>
              <input v-model.number="form.limitBurst" type="number" min="0" />
            </label>
            <label class="stack-field">
              <span>{{ t('dialog.site.limitMem') }}</span>
              <input v-model.number="form.limitMem" type="number" min="0" />
            </label>
          </div>
          <div class="check-row">
            <label class="check-field">
              <input v-model="form.limitNodelay" type="checkbox" />
              <span>{{ t('dialog.site.limitNodelay') }}</span>
            </label>
          </div>
          <small class="field-note">{{ t('dialog.site.limitHint') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.auth') }}</div>
          <div class="check-row">
            <label class="check-field">
              <input v-model="form.authOn" type="checkbox" />
              <span>{{ t('dialog.site.authOn') }}</span>
            </label>
          </div>
          <div v-if="form.authOn" class="adv__fields adv__fields--2">
            <label class="stack-field">
              <span>{{ t('dialog.site.authUser') }}</span>
              <input v-model="form.authUser" type="text" autocomplete="off" />
            </label>
            <label class="stack-field">
              <span>{{ t('dialog.site.authPass') }}</span>
              <input v-model="form.authPass" type="password" autocomplete="new-password" />
            </label>
            <label class="stack-field span-2">
              <span>{{ t('dialog.site.authRealm') }}</span>
              <input v-model="form.authRealm" type="text" />
            </label>
          </div>
          <small class="field-note">{{ t('dialog.site.authHint') }}</small>
        </div>

        <div class="adv__block">
          <div class="adv__title">{{ t('dialog.site.cache') }}</div>
          <label class="stack-field">
            <span>{{ t('dialog.site.cacheZone') }}</span>
            <select v-model="form.cacheZone">
              <option value="">{{ t('dialog.site.cacheOff') }}</option>
              <option v-for="z in zones" :key="z.name" :value="z.name">
                {{ z.name }} · {{ z.path }}
              </option>
            </select>
          </label>
          <small class="field-note">{{ t('dialog.site.cacheZoneHint') }}</small>
          <small v-if="zones.length === 0" class="field-note">{{ t('dialog.site.cacheNone') }}</small>
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

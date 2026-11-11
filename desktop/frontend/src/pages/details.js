// 明细渲染：站点、证书、运行时路径，以及状态矩阵里的访问日志栏。
// 站点行来自 ListSites()（model.Site：id/name/port/listen/listens/tls/source/
// enabled/error），**一个站点文件一行**；行内按钮用 data-id（站点名，
// site 命令的位置参数）。证书的有效期与状态徽标只在证书页展示，站点页与
// 矩阵只提示签发来源。
import { $, esc } from '../ui/dom.js';
import { iconSVG } from '../ui/icons.js';
import { t } from '../i18n.js';
import { state } from '../state.js';

// basename 取路径最后一段（站点配置文件显示成 localhost.json）。
function basename(p) {
  if (!p) return '';
  return String(p).split(/[\\/]/).pop();
}

export function renderRows(target, rows) {
  $(target).innerHTML = rows.length
    ? rows.map((r) => `<div class="kv-row"><span class="k">${esc(r.k)}</span><span class="v">${esc(r.v)}</span><span class="n">${esc(r.n || '')}</span></div>`).join('')
    : `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('common.none'))}</span></div>`;
}

function formatAccessSize(n) {
  if (!Number.isFinite(n)) return '—';
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / (1024 * 1024)).toFixed(1) + ' MB';
}

// model.LogEntry.Size 是字符串：纯数字按字节格式化，否则原样显示
function sizeText(size) {
  const s = String(size ?? '').trim();
  if (!s) return '—';
  return /^\d+$/.test(s) ? formatAccessSize(Number(s)) : s;
}

function timeHHMMSS(time) {
  const m = String(time || '').match(/T(\d{2}:\d{2}:\d{2})/);
  return m ? m[1] : (String(time || '').slice(0, 8) || '—');
}

// 证书状态徽标：valid 绿 / expiring 橙 / expired、missing 红 / 其余中性
const CERT_STATUS = {
  valid: ['ok', 'certs.stValid'],
  expiring: ['warn', 'certs.stExpiring'],
  expired: ['err', 'certs.stExpired'],
  not_yet_valid: ['off', 'certs.stNotYetValid'],
  missing: ['err', 'certs.stMissing'],
  unreadable: ['err', 'certs.stUnreadable'],
  unknown: ['off', 'certs.stUnknown']
};

export function certBadge(status) {
  const st = String(status || '');
  if (!st) return '';
  const [cls, key] = CERT_STATUS[st] || ['off', ''];
  return `<span class="st-badge st-${cls}" title="${esc(st)}">${esc(key ? t(key) : st)}</span>`;
}

// 状态矩阵「访问日志」栏：TailLog('access', n) 现在返回 model.LogEntry[]，
// 不再解析 JSON 行；tooltip 用 raw（记录的紧凑 JSON）。
export function renderLogEntries(entries, target) {
  const el = $(target);
  if (!el) return;
  if (!entries || entries.length === 0) {
    el.innerHTML = `<div class="log-empty">${esc(t('matrix.accessEmpty'))}</div>`;
    return;
  }
  el.innerHTML = entries.slice(0, 8).map((e) => {
    const status = String(e.status ?? '') || '—';
    const code = Number(status);
    const cls = code >= 400 ? 'err' : (code >= 300 ? 'warn' : (Number.isFinite(code) ? 'ok' : ''));
    return `<div class="access-row" title="${esc(e.raw || '')}">`
      + `<span class="a-time">${esc(timeHHMMSS(e.time))}</span>`
      + `<span class="a-method">${esc(e.method || '—')}</span>`
      + `<span class="a-path" title="${esc(e.path || e.message || '')}">${esc(e.path || e.message || '—')}</span>`
      + `<span class="a-status ${cls}">${esc(status)}</span>`
      + `<span class="a-size">${esc(sizeText(e.size))}</span></div>`;
  }).join('');
}

// 站点访问地址：优先 https（站点带 ssl 时），端口按监听里对应协议的端口取。
// 一个站点可能同时监听 80 和 443，这里挑最能代表它的那个：
// 有 ssl 监听就用 443 那条，否则用第一条。
export function siteURL(s) {
  const listens = Array.isArray(s.listens) && s.listens.length ? s.listens : [s.listen || ''];
  let picked = listens[0] || '';
  if (s.tls) {
    const ssl = listens.find((l) => /ssl/i.test(l));
    if (ssl) picked = ssl;
  }
  const scheme = /ssl/i.test(picked) ? 'https' : 'http';
  const port = String(picked).trim().split(/\s+/)[0].replace(/^:/, '');
  const host = s.name || 'localhost';
  const isDefault = (scheme === 'https' && port === '443') || (scheme === 'http' && port === '80');
  return scheme + '://' + host + (port && !isDefault ? ':' + port : '') + '/';
}

// listenChips 把站点的每条监听渲染成一个小标签：80 / 443 ssl h2。
function listenChips(s) {
  const listens = Array.isArray(s.listens) && s.listens.length ? s.listens : (s.listen ? [s.listen] : []);
  if (!listens.length) return '<span class="listen-chip">—</span>';
  return listens.map((l) => {
    const secure = /ssl/i.test(l);
    return `<span class="listen-chip${secure ? ' is-tls' : ''}">${esc(l)}</span>`;
  }).join('');
}

export function renderSites(sites, certs) {
  $('#siteCountLabel').textContent = sites.length;
  $('#sitesTable').innerHTML = sites.length
    ? sites.map((s) => {
        const url = siteURL(s);
        const off = state.lastOverview?.running ? '' : ' is-off';
        const isOn = s.enabled !== false;
        const toggleBtn = '<button class="icon-btn site-toggle' + (isOn ? ' is-on' : ' is-off') + '"'
          + ' data-id="' + esc(s.id) + '"'
          + ' data-enabled="' + (isOn ? '0' : '1') + '"'
          + ' title="' + esc(isOn ? t('sites.toggleOn') : t('sites.toggleOff')) + '">'
          + iconSVG('power')
          + '</button>';
        const delBtn = '<button class="icon-btn site-del" data-id="' + esc(s.id) + '" data-name="' + esc(s.name) + '" title="' + esc(t('sites.del')) + '">'
          + iconSVG('trash')
          + '</button>';
        const detailBtn = '<button class="icon-btn site-more" data-id="' + esc(s.id) + '" title="' + esc(t('sites.detail')) + '">' + iconSVG('more') + '</button>';
        const err = s.error ? '<span class="row-err" title="' + esc(s.error) + '">' + esc(s.error) + '</span>' : '';
        // 一个站点文件一行：主行给访问地址与开关，下面一行列全部监听标签。
        // 不再按监听拆成多行——那是同一个文件、同一个开关，拆开会让用户
        // 以为有两个站点，且点其中一个另一个也跟着变。
        const statePill = '<span class="site-state ' + (isOn ? 'on' : 'off') + '">'
          + esc(isOn ? t('sites.stateOn') : t('sites.stateOff')) + '</span>';
        return `<div class="site-row${isOn ? '' : ' is-disabled'}">`
          + `<div class="site-main">`
          + `<a class="site-link${off}" href="#" data-url="${esc(url)}" title="${esc(url)}">${esc(url)}</a>`
          + statePill
          + `<span class="site-file" title="${esc(s.source || '')}">${esc(basename(s.source) || s.id)}</span>`
          + err
          + `</div>`
          + `<div class="site-side">`
          + `<div class="site-listens">${listenChips(s)}</div>`
          + `<div class="site-actions">${toggleBtn}${delBtn}${detailBtn}</div>`
          + `</div>`
          + `</div>`;
      }).join('')
    : `<div class="site-empty">${esc(t('sites.empty'))}</div>`;
  $('#matrixBody').innerHTML = sites.length
    ? sites.map((s) => {
        const c = certs.find((x) => x.domain === s.name);
        let dot = 'off', text = t('matrix.legendOff');
        if (state.lastOverview?.running) {
          if (c?.local || c?.selfSigned) { dot = 'warn'; text = t('matrix.localCert'); }
          else { dot = 'ok'; text = t('matrix.legendOk'); }
        }
        // 证书列只提示签发来源（详情与有效期在证书页看），不放有效/到期徽标
        const issuer = c ? (c.local ? t('certs.local') : (c.issuer || '—')) : '—';
        return `<tr>
          <td title="${esc(s.name)}">${esc(s.name)}</td>
          <td class="mono">${esc(s.port || '—')}</td>
          <td><span class="tls-pill ${s.tls ? 'on' : ''}">${s.tls ? 'ssl' : 'HTTP'}</span></td>
          <td class="cert" title="${esc(issuer)}">${esc(issuer)}</td>
          <td class="state-cell"><span class="matrix-state"><i class="dot ${dot}"></i>${text}</span></td>
        </tr>`;
      }).join('')
    : `<tr><td colspan="5">${esc(t('sites.empty'))}</td></tr>`;
}

export function renderCerts(certs) {
  $('#certsTable').innerHTML = certs.length
    ? certs.map((c) => {
        const detail = [
          c.subject || c.domain,
          c.local ? t('certs.local') : (c.issuer || '—'),
          c.notAfter || '—',
          c.daysLeft > 0 ? t('certs.daysLeft', { days: c.daysLeft }) : (c.status === 'expired' ? t('certs.stExpired') : ''),
          c.names || '',
          c.note || ''
        ].filter(Boolean).join('　·　');
        return `<div class="kv-row">`
          + `<span class="k">${esc(c.domain)}</span>`
          + `<span class="v">${esc(detail)}</span>`
          + `<span class="n with-action">${certBadge(c.status) || esc(c.local ? t('certs.localGenerated') : (c.selfSigned ? t('certs.selfSigned') : t('certs.issued')))}`
          + `<button class="icon-btn cert-check" data-domain="${esc(c.domain)}" title="${esc(t('certs.check'))}">`
          + iconSVG('search')
          + `</button></span></div>`;
      }).join('')
    : `<div class="kv-row"><span class="k">—</span><span class="v">${esc(t('certs.empty'))}</span></div>`;
  $('#metricCerts').textContent = certs.length;
  $('#certCount').textContent = certs.length;
}

export function renderPaths(p) {
  renderRows('#pathsTable', [
    { k: t('paths.config'), v: p.config, n: 'config' },
    { k: t('paths.cert'), v: p.cert, n: 'cert' },
    { k: t('paths.log'), v: p.log, n: 'log' },
    { k: t('paths.pid'), v: p.pid, n: 'pid' },
    { k: t('paths.state'), v: p.state, n: 'state' }
  ]);
}

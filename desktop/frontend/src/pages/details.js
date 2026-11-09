// 明细渲染：站点、证书、运行时路径，以及状态矩阵里的访问日志栏。
// 站点行来自 ListSites()（model.Site：id/name/port/listen/tls/source/enabled/
// certStatus/error），行内按钮用 data-id（站点名，site 命令的位置参数）。
import { $, esc } from '../ui/dom.js';
import { state } from '../state.js';

export function renderRows(target, rows) {
  $(target).innerHTML = rows.length
    ? rows.map((r) => `<div class="kv-row"><span class="k">${esc(r.k)}</span><span class="v">${esc(r.v)}</span><span class="n">${esc(r.n || '')}</span></div>`).join('')
    : '<div class="kv-row"><span class="k">—</span><span class="v">没有可显示的数据</span></div>';
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

function timeHHMMSS(t) {
  const m = String(t || '').match(/T(\d{2}:\d{2}:\d{2})/);
  return m ? m[1] : (String(t || '').slice(0, 8) || '—');
}

// 证书状态徽标：valid 绿 / expiring 橙 / expired、missing 红 / 其余中性
const CERT_STATUS = {
  valid: ['ok', '有效'],
  expiring: ['warn', '即将到期'],
  expired: ['err', '已过期'],
  not_yet_valid: ['off', '未生效'],
  missing: ['err', '缺失'],
  unreadable: ['err', '不可读'],
  unknown: ['off', '未知']
};

export function certBadge(status) {
  const st = String(status || '');
  if (!st) return '';
  const [cls, label] = CERT_STATUS[st] || ['off', st];
  return `<span class="st-badge st-${cls}" title="${esc(st)}">${esc(label)}</span>`;
}

// 状态矩阵「访问日志」栏：TailLog('access', n) 现在返回 model.LogEntry[]，
// 不再解析 JSON 行；tooltip 用 raw（记录的紧凑 JSON）。
export function renderLogEntries(entries, target) {
  const el = $(target);
  if (!el) return;
  if (!entries || entries.length === 0) {
    el.innerHTML = '<div class="log-empty">日志为空或文件不存在</div>';
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

// 站点访问地址：默认端口省略，其他端口显式带上；name 是站点域名（server_name）
export function siteURL(s) {
  const scheme = s.tls ? 'https' : 'http';
  const port = String(s.port || '').replace(/^:/, '');
  const host = s.name || 'localhost';
  const isDefault = (scheme === 'https' && port === '443') || (scheme === 'http' && port === '80');
  return scheme + '://' + host + (port && !isDefault ? ':' + port : '') + '/';
}

export function renderSites(sites, certs) {
  $('#siteCountLabel').textContent = sites.length;
  $('#sitesTable').innerHTML = sites.length
    ? sites.map((s) => {
        const url = siteURL(s);
        const off = state.lastOverview?.running ? '' : ' is-off';
        // site list 的每行都是 conf.d 下的独立站点文件，全部可启停/删除
        const isOn = s.enabled !== false;
        const toggleBtn = '<button class="icon-btn site-toggle' + (isOn ? '' : ' is-off') + '"'
          + ' data-id="' + esc(s.id) + '"'
          + ' data-enabled="' + (isOn ? '0' : '1') + '"'
          + ' title="' + (isOn ? '停用站点' : '启用站点') + '">'
          + '<svg viewBox="0 0 16 16" aria-hidden="true">'
          + '<path d="M8 2.2v5.1M4.6 4.1a5 5 0 1 0 6.8 0"/>'
          + (isOn ? '' : '<path d="M2.4 2.4l11.2 11.2"/>')
          + '</svg></button>';
        const delBtn = '<button class="icon-btn site-del" data-id="' + esc(s.id) + '" data-name="' + esc(s.name) + '" title="删除站点">'
          + '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2.6 4.4h10.8M6.3 4.4V2.8h3.4v1.6M3.9 4.4l.5 8.1c.04.6.55 1.1 1.16 1.1h4.88c.61 0 1.12-.5 1.16-1.1l.5-8.1M6.6 7.2v3.8M9.4 7.2v3.8"/></svg>'
          + '</button>';
        const detailBtn = '<button class="icon-btn site-more" data-id="' + esc(s.id) + '" title="查看详情">⋯</button>';
        const badge = certBadge(s.certStatus);
        const err = s.error ? '<span class="row-err" title="' + esc(s.error) + '">' + esc(s.error) + '</span>' : '';
        return `<div class="kv-row">`
          + `<span class="k">${esc(s.port || '—')}</span>`
          + `<span class="v"><a class="site-link${off}" href="#" data-url="${esc(url)}" title="${esc(url)}">${esc(url)}</a>${badge}</span>`
          + `<span class="n with-action"><span title="${esc(s.source || '')}">${esc(s.name || s.source || '')}</span>${err}${toggleBtn}${delBtn}${detailBtn}</span>`
          + `</div>`;
      }).join('')
    : '<div class="kv-row"><span class="k">—</span><span class="v">没有站点</span></div>';
  $('#matrixBody').innerHTML = sites.length
    ? sites.map((s) => {
        const c = certs.find((x) => x.domain === s.name);
        let dot = 'off', text = '未运行';
        if (state.lastOverview?.running) {
          if (c?.local || c?.selfSigned) { dot = 'warn'; text = '本地证书'; }
          else { dot = 'ok'; text = '正常'; }
        }
        const issuer = c ? (c.local ? '本地 CA 自动签发' : (c.issuer || '—')) : (s.certStatus || '—');
        return `<tr>
          <td title="${esc(s.name)}">${esc(s.name)}</td>
          <td class="mono">${esc(s.port || '—')}</td>
          <td><span class="tls-pill ${s.tls ? 'on' : ''}">${s.tls ? 'ssl' : 'HTTP'}</span></td>
          <td class="cert" title="${esc(issuer)}">${certBadge(s.certStatus) || esc(issuer)}</td>
          <td class="state-cell"><span class="matrix-state"><i class="dot ${dot}"></i>${text}</span></td>
        </tr>`;
      }).join('')
    : '<tr><td colspan="5">没有站点</td></tr>';
}

export function renderCerts(certs) {
  $('#certsTable').innerHTML = certs.length
    ? certs.map((c) => {
        const detail = [
          c.subject || c.domain,
          c.local ? '本地 CA 自动签发' : (c.issuer || '—'),
          c.notAfter || '—',
          c.daysLeft > 0 ? '剩 ' + c.daysLeft + ' 天' : (c.status === 'expired' ? '已过期' : ''),
          c.names || '',
          c.note || ''
        ].filter(Boolean).join('　·　');
        return `<div class="kv-row">`
          + `<span class="k">${esc(c.domain)}</span>`
          + `<span class="v">${esc(detail)}</span>`
          + `<span class="n with-action">${certBadge(c.status) || esc(c.local ? '本地生成' : (c.selfSigned ? '自签' : '已签发'))}`
          + `<button class="icon-btn cert-check" data-domain="${esc(c.domain)}" title="检查该域名的证书">`
          + `<svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="7" cy="7" r="4.4"/><path d="M10.4 10.4L14 14"/></svg>`
          + `</button></span></div>`;
      }).join('')
    : '<div class="kv-row"><span class="k">—</span><span class="v">没有证书</span></div>';
  $('#metricCerts').textContent = certs.length;
  $('#certCount').textContent = certs.length;
}

export function renderPaths(p) {
  renderRows('#pathsTable', [
    { k: '主配置', v: p.config, n: 'config' },
    { k: '证书目录', v: p.cert, n: 'cert' },
    { k: '日志目录', v: p.log, n: 'log' },
    { k: 'pid 文件', v: p.pid, n: 'pid' },
    { k: '管理状态目录', v: p.state, n: 'state' }
  ]);
}

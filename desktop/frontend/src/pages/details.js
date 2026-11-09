// 明细渲染：站点、证书、运行时路径，以及状态矩阵里的访问日志栏。
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

// access.log 每行是结构化 JSON；矩阵里只取时间/方法/路径/状态/大小，
// 不再把原始 JSON 整串折行塞进窄栏。非 JSON 行退化成截断文本。
function parseAccessLine(line) {
  let o;
  try { o = JSON.parse(line); } catch (e) { return null; }
  if (!o || typeof o !== 'object') return null;
  const t = String(o.time || '');
  const m = t.match(/T(\d{2}:\d{2}:\d{2})/);
  return {
    time: m ? m[1] : (t.slice(0, 8) || '—'),
    method: String(o.method || '—'),
    path: String(o.path || '—'),
    status: o.status != null ? String(o.status) : '—',
    size: o.size != null ? formatAccessSize(Number(o.size)) : '—'
  };
}

export function renderLogs(lines, target, kind) {
  const el = $(target);
  if (!lines || lines.length === 0) {
    el.innerHTML = kind === 'rail' ? '<div class="log-empty">日志为空或文件不存在</div>' : '<tr><td colspan="3">日志为空或文件不存在</td></tr>';
    return;
  }
  if (kind === 'rail') {
    el.innerHTML = lines.slice(0, 8).map((line) => {
      const a = parseAccessLine(line);
      if (!a) {
        return `<div class="access-row"><span class="a-time">—</span><span class="a-method">—</span><span class="a-path" title="${esc(line)}">${esc(line.slice(0, 90))}</span><span class="a-status">—</span><span class="a-size">—</span></div>`;
      }
      const code = Number(a.status);
      const cls = code >= 400 ? 'err' : (code >= 300 ? 'warn' : (Number.isFinite(code) ? 'ok' : ''));
      return `<div class="access-row"><span class="a-time">${esc(a.time)}</span><span class="a-method">${esc(a.method)}</span><span class="a-path" title="${esc(a.path)}">${esc(a.path)}</span><span class="a-status ${cls}">${esc(a.status)}</span><span class="a-size">${esc(a.size)}</span></div>`;
    }).join('');
    return;
  }
  el.innerHTML = lines.map((l) => `<tr><td colspan="3" class="mono">${esc(l)}</td></tr>`).join('');
}

// 站点访问地址：默认端口省略，其他端口显式带上；server_name 可能是域名或本机 IP。
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
        // 只有 conf.d 下的站点文件可删；主配置内联站点没有独立文件。
        // 归一化反斜杠：Windows 下 Source 可能是 conf.d\x.json 的形式。
        const rel = String(s.source || '').replace(/\\/g, '/');
        const canToggle = rel.startsWith('conf.d/') &&
          (rel.endsWith('.json') || rel.endsWith('.json.disabled'));
        const isOn = s.enabled !== false;
        const toggleBtn = canToggle
          ? '<button class="icon-btn site-toggle' + (isOn ? '' : ' is-off') + '"'
            + ' data-source="' + esc(s.source) + '"'
            + ' data-enabled="' + (isOn ? '0' : '1') + '"'
            + ' title="' + (isOn ? '停用站点' : '启用站点') + '">'
            + '<svg viewBox="0 0 16 16" aria-hidden="true">'
            + '<path d="M8 2.2v5.1M4.6 4.1a5 5 0 1 0 6.8 0"/>'
            + (isOn ? '' : '<path d="M2.4 2.4l11.2 11.2"/>')
            + '</svg></button>'
          : '';
        const delBtn = rel.startsWith('conf.d/')
          ? '<button class="icon-btn site-del" data-source="' + esc(s.source) + '" title="删除站点">'
            + '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2.6 4.4h10.8M6.3 4.4V2.8h3.4v1.6M3.9 4.4l.5 8.1c.04.6.55 1.1 1.16 1.1h4.88c.61 0 1.12-.5 1.16-1.1l.5-8.1M6.6 7.2v3.8M9.4 7.2v3.8"/></svg>'
            + '</button>'
          : '';
        return `<div class="kv-row"><span class="k">${esc(s.port || '—')}</span><span class="v"><a class="site-link${off}" href="#" data-url="${esc(url)}" title="${esc(url)}">${esc(url)}</a></span><span class="n with-action"><span>${esc(s.source || '')}</span>${toggleBtn}${delBtn}<button class="icon-btn site-more" data-source="${esc(s.source || '')}" title="查看详情">⋯</button></span></div>`;
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
        const issuer = c ? (c.local ? '本地 CA 自动签发' : (c.issuer || '—')) : '—';
        return `<tr>
          <td title="${esc(s.name)}">${esc(s.name)}</td>
          <td class="mono">${esc(s.port || '—')}</td>
          <td><span class="tls-pill ${s.tls ? 'on' : ''}">${s.tls ? 'ssl' : 'HTTP'}</span></td>
          <td class="cert" title="${esc(issuer)}">${esc(issuer)}</td>
          <td class="state-cell"><span class="matrix-state"><i class="dot ${dot}"></i>${text}</span></td>
        </tr>`;
      }).join('')
    : '<tr><td colspan="5">没有站点</td></tr>';
}

export function renderCerts(certs) {
  renderRows('#certsTable', certs.map((c) => ({
    k: c.domain,
    v: [
      c.subject || c.domain,
      c.local ? '本地 CA 自动签发' : (c.issuer || '—'),
      c.notAfter || '—',
      c.daysLeft > 0 ? '剩 ' + c.daysLeft + ' 天' : '已过期',
      c.names || ''
    ].filter(Boolean).join('　·　'),
    n: c.local ? '本地生成' : (c.selfSigned ? '自签' : '已签发')
  })));
  $('#metricCerts').textContent = certs.length;
  $('#certCount').textContent = certs.length;
}

export function renderPaths(p) {
  renderRows('#pathsTable', [
    { k: '主配置', v: p.config, n: 'config' },
    { k: '证书目录', v: p.cert, n: 'cert' },
    { k: '日志目录', v: p.log, n: 'log' },
    { k: 'pid 文件', v: p.pid, n: 'pid' }
  ]);
}

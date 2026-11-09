// 命令面板：把 CLI 的命令树直接铺到界面上。
// 清单由 Go 侧 ListCommands() 提供，与 CLI 一一对应；加命令只改 Go 的表，
// 这里不用跟着改。所有执行都走 RunCommand → cli.RunJSON，桌面端不碰文件。
import { DesktopService, msgOf } from '../api.js';
import { $, esc, toast } from '../ui/dom.js';

const BOOL_FLAGS = new Set(['--tls', '--yes', '--dry-run', '--merged']);
// 可重复 flag：逗号分隔的多个值各占一个 --flag 位置（--listen 80 --listen "443 ssl h2"）
const REPEATABLE_FLAGS = new Set(['--listen']);
const VALUE_HINTS = {
  '--domain': 'blog.example',
  '--listen': '80, 443 ssl h2',
  '--root': './httpdoc/root',
  '--proxy-pass': 'http://127.0.0.1:8080',
  '--server-name': 'a.example b.example',
  '--config': '留空用默认路径',
  '--candidate': '候选配置目录',
  '--current': '当前配置目录或文件',
  '--version': 'v001',
  '--section': 'http|acme|security|waf|sites',
  '--id': '操作 ID',
  '--limit': '20',
  '--level': 'info',
  '--status': '4xx 或 500',
  '--kind': 'access|error|system|safe',
  '--until': '10m',
  '--since': '1h',
  '--severity': 'info|warning|error|critical',
  '--method': 'GET',
  '--path-prefix': '/api',
  '--reason': 'blacklist',
};

let specs = [];
let current = -1;

function renderList() {
  const groups = [...new Set(specs.map((s) => s.group))];
  $('#commandList').innerHTML = groups.map((g) =>
    '<div class="cmd-group"><h4>' + esc(g) + '</h4>' +
    specs.map((s, i) => i).filter((i) => specs[i].group === g)
      .map((i) => '<button class="cmd-item" data-index="' + i + '">' +
        esc(specs[i].label) + '<small>' + esc(specs[i].name) + '</small></button>')
      .join('') +
    '</div>').join('');
}

function fieldFor(flag) {
  const id = 'cf' + flag.replace(/[^a-z]/gi, '');
  const bool = BOOL_FLAGS.has(flag);
  const input = bool
    ? '<input type="checkbox" id="' + id + '" data-flag="' + flag + '" data-bool="1"/>'
    : '<input type="text" id="' + id + '" data-flag="' + flag + '" placeholder="' +
      esc(VALUE_HINTS[flag] || '') + '"/>';
  return '<div class="form-row ' + (bool ? 'cmd-check' : '') + '">' +
    '<label class="form-label" for="' + id + '">' + esc(flag) + '</label>' +
    '<div class="form-control">' + input + '</div></div>';
}

function renderForm(i) {
  current = i;
  const s = specs[i];
  if (!s) return;
  $('#cmdFormTitle').textContent = s.label + ' · ' + s.name;
  $('#cmdForm').innerHTML =
    (s.args && s.args.length ? s.args.map((a) =>
      '<div class="form-row"><label class="form-label">位置参数</label>' +
      '<div class="form-control"><input type="text" data-positional placeholder="' +
      esc(a) + '"/></div></div>').join('') :
      '<div class="form-row"><label class="form-label">位置参数（站点名 / 操作 ID 等）</label>' +
      '<div class="form-control"><input type="text" id="cmdPositional" data-positional placeholder="按需填写"/></div></div>') +
    (s.flags || []).map(fieldFor).join('') +
    '<div class="form-row cmd-check"><label class="form-label" for="cmdConfirm">确认执行</label>' +
    '<div class="form-control"><input type="checkbox" id="cmdConfirm"' +
    (s.needsYes ? '' : ' checked') + '/><small>写操作需要勾选，等价于 CLI 的 --yes</small></div></div>';
  $('#cmdResult').textContent = '选择一条命令并点击「执行」。';
  $('#cmdResult').className = 'cmd-result is-empty';
  document.querySelectorAll('.cmd-item').forEach((b) =>
    b.classList.toggle('on', Number(b.getAttribute('data-index')) === i));
}

// 去掉用户粘贴时带的一层引号，保留值内部的空格
function stripQuotes(v) {
  const m = v.match(/^(['"])([\s\S]*)\1$/);
  return (m ? m[2] : v).trim();
}

function collectArgs(s) {
  const parts = s.name.split(' ').filter(Boolean);
  document.querySelectorAll('#cmdForm [data-positional]').forEach((el) => {
    const v = el.value.trim();
    if (v) parts.push(...v.split(/\s+/));
  });
  document.querySelectorAll('#cmdForm [data-flag]').forEach((el) => {
    const flag = el.getAttribute('data-flag');
    if (el.dataset.bool) {
      if (el.checked) parts.push(flag);
      return;
    }
    const v = stripQuotes(el.value);
    if (!v) return;
    if (REPEATABLE_FLAGS.has(flag)) {
      v.split(',').map((x) => x.trim()).filter(Boolean)
        .forEach((item) => parts.push(flag, item));
    } else {
      // 单值 flag 整体作为一个 argv：值内允许空格（--server-name "a.example b.example"）
      parts.push(flag, v);
    }
  });
  const confirm = $('#cmdConfirm');
  if (confirm && !confirm.checked) {
    // 未确认的写操作一律加 --dry-run，避免误改
    if (s.needsYes && !parts.includes('--dry-run')) parts.push('--dry-run');
  }
  return parts;
}

async function run() {
  const s = specs[current];
  if (!s) { toast('请先选一条命令'); return; }
  const args = collectArgs(s);
  const btn = $('#cmdRun');
  btn.disabled = true;
  $('#cmdResult').className = 'cmd-result';
  $('#cmdResult').textContent = '执行中：' + args.join(' ');
  try {
    const res = await DesktopService.RunCommand(args);
    // TextOnly 命令（version/start/stop/reload/status/acme show/config path）
    // 没有 JSON 封套，结果在 res.text；其余展示 data 原样 JSON
    const body = res.text !== undefined && res.text !== ''
      ? res.text
      : JSON.stringify(res.data ?? {}, null, 2);
    const warns = (res.warnings || []).length
      ? '\n\n提示：\n' + res.warnings.join('\n') : '';
    $('#cmdResult').textContent = body + warns;
  } catch (err) {
    $('#cmdResult').className = 'cmd-result is-error';
    $('#cmdResult').textContent = '失败：' + msgOf(err);
  } finally {
    btn.disabled = false;
  }
}

export async function initCommandPanel() {
  if (!$('#commandList')) return;
  try {
    specs = await DesktopService.ListCommands();
    renderList();
    $('#commandList').addEventListener('click', (e) => {
      const b = e.target.closest('.cmd-item');
      if (b) renderForm(Number(b.getAttribute('data-index')));
    });
    $('#cmdRun').addEventListener('click', run);
    if (specs.length) renderForm(0);
  } catch (err) {
    $('#commandList').textContent = '读取命令清单失败：' + msgOf(err);
  }
}

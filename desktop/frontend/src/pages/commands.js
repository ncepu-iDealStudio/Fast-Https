// 命令面板：把 CLI 的命令树直接铺到界面上。
// 清单由 Go 侧 ListCommands() 提供，与 CLI 一一对应；加命令只改 Go 的表，
// 这里不用跟着改。所有执行都走 RunCommand → cli.RunJSON，桌面端不碰文件。
// 命令名/参数名/示例值是语言无关的 CLI 文本，不翻译；界面提示语走 t()。
import { DesktopService, msgOf } from '../api.js';
import { t } from '../i18n.js';
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
  '--version': 'v001',
  '--section': 'http|acme|security|waf|sites',
  '--limit': '20',
  '--level': 'info',
  '--kind': 'access|error|system|safe',
  '--until': '10m',
  '--since': '1h',
  '--severity': 'info|warning|error|critical',
  '--method': 'GET',
  '--path-prefix': '/api',
  '--reason': 'blacklist',
};
// 提示语是界面文案，按当前语言取
const HINT_KEYS = {
  '--config': 'cmd.hintConfig',
  '--candidate': 'cmd.hintCandidate',
  '--current': 'cmd.hintCurrent',
  '--id': 'cmd.hintId',
  '--status': 'cmd.hintStatus',
};

function hintFor(flag) {
  return HINT_KEYS[flag] ? t(HINT_KEYS[flag]) : (VALUE_HINTS[flag] || '');
}

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
      esc(hintFor(flag)) + '"/>';
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
      '<div class="form-row"><label class="form-label">' + esc(t('cmd.positional')) + '</label>' +
      '<div class="form-control"><input type="text" data-positional placeholder="' +
      esc(a) + '"/></div></div>').join('') :
      '<div class="form-row"><label class="form-label">' + esc(t('cmd.positionalHint')) + '</label>' +
      '<div class="form-control"><input type="text" id="cmdPositional" data-positional placeholder="' + esc(t('cmd.positionalPh')) + '"/></div></div>') +
    (s.flags || []).map(fieldFor).join('') +
    '<div class="form-row cmd-check"><label class="form-label" for="cmdConfirm">' + esc(t('cmd.confirm')) + '</label>' +
    '<div class="form-control"><input type="checkbox" id="cmdConfirm"' +
    (s.needsYes ? '' : ' checked') + '/><small>' + esc(t('cmd.confirmHint')) + '</small></div></div>';
  $('#cmdResult').textContent = t('cmd.resultEmpty');
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
  if (!s) { toast(t('cmd.selectFirst')); return; }
  const args = collectArgs(s);
  const btn = $('#cmdRun');
  btn.disabled = true;
  $('#cmdResult').className = 'cmd-result';
  $('#cmdResult').textContent = t('cmd.running', { args: args.join(' ') });
  try {
    const res = await DesktopService.RunCommand(args);
    // TextOnly 命令（version/start/stop/reload/status/acme show/config path）
    // 没有 JSON 封套，结果在 res.text；其余展示 data 原样 JSON
    const body = res.text !== undefined && res.text !== ''
      ? res.text
      : JSON.stringify(res.data ?? {}, null, 2);
    const warns = (res.warnings || []).length
      ? '\n\n' + t('cmd.warnings') + '\n' + res.warnings.join('\n') : '';
    $('#cmdResult').textContent = body + warns;
  } catch (err) {
    $('#cmdResult').className = 'cmd-result is-error';
    $('#cmdResult').textContent = t('cmd.failed', { msg: msgOf(err) });
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
    $('#commandList').textContent = t('cmd.listFailed', { msg: msgOf(err) });
  }
}

/**
 * Monitor 事件的「人话」翻译层。
 *
 * 后端写入的事件把 Go 的网络错误原样存进了 message，例如：
 *   unreachable: Get "http://127.0.0.1:9088/runtime": dial tcp ...: connectex:
 *   No connection could be made because the target machine actively refused it.;
 *   server not started (no pid file)
 * 这种文本对用户毫无意义。这里按 type/rule/severity 归类，翻译成一句能看懂的话，
 * 并把可量化的变化（5xx 增量、请求增量、PID）放进 detail。
 *
 * 真实 type 取值：collect_error / unreachable / recovered / 5xx / started / stopped …
 */

export interface RawEvent {
  time?: string;
  type?: string;
  rule?: string;
  severity?: string;
  message?: string;
  snapshot?: {
    state?: string;
    pid?: number;
    requests_total?: number;
    responses_5xx_total?: number;
    requests_delta?: number;
    responses_5xx_delta?: number;
  };
}

export interface HumanEvent {
  title: string;
  detail: string;
  tone: 'ok' | 'warn' | 'info';
  time: string;
}

type T = (key: string, named?: Record<string, unknown>) => string;

/** 严重级别 → 色调。error/critical 用 warn 色（红），warning 也用 warn，正常用 ok。 */
function toneOf(sev?: string, type?: string): 'ok' | 'warn' | 'info' {
  const s = `${sev ?? ''} ${type ?? ''}`.toLowerCase();
  if (/crit|error|fatal|alert/.test(s)) return 'warn';
  if (/warn/.test(s)) return 'warn';
  if (/ok|recover|info|normal/.test(s)) return 'ok';
  return 'info';
}

/** 从 message 提取「连续失败次数」这类可量化信息。 */
function failCount(msg: string): number | null {
  const m = msg.match(/(\d+)\s*consecutive\s*(?:collection\s*)?failures/i);
  return m ? Number(m[1]) : null;
}

/** 把一条原始事件翻译成用户能看懂的一句话。 */
export function humanizeEvent(e: RawEvent, t: T): HumanEvent {
  const raw = e.time ?? '';
  const tm = raw.match(/(\d{2}:\d{2}:\d{2})/);
  const time = tm ? tm[1] : raw.replace('T', ' ').slice(0, 16);

  const snap = e.snapshot ?? {};
  const rule = `${e.rule ?? ''} ${e.type ?? ''}`.toLowerCase();
  const msg = e.message ?? '';
  const tone = toneOf(e.severity, e.type);

  // 采集失败 / 连不上：都是「服务没起来，探针访问不到」
  if (/collect_error|unreachable|offline|down|refused/.test(rule) || /unreachable|refused/i.test(msg)) {
    const n = failCount(msg);
    return {
      title: t('evt.collectFailed'),
      detail: n && n > 1 ? t('evt.collectFailedRepeat', { n }) : t('evt.collectFailedDetail'),
      tone: 'warn', time,
    };
  }

  if (/recover|resume|back|healthy/.test(rule)) {
    return { title: t('evt.serviceRecovered'), detail: t('evt.serviceRecoveredDetail'), tone: 'ok', time };
  }
  if (/5xx/.test(rule)) {
    const d = snap.responses_5xx_delta;
    return {
      title: t('evt.serverErrors'),
      detail: typeof d === 'number' && d > 0 ? t('evt.serverErrorsDetail', { n: d }) : t('evt.serverErrorsDetailPlain'),
      tone: 'warn', time,
    };
  }
  if (/latency|slow|timeout/.test(rule)) {
    return { title: t('evt.slowResponse'), detail: t('evt.slowResponseDetail'), tone: 'warn', time };
  }
  if (/start|boot|launch/.test(rule)) {
    return { title: t('evt.started'), detail: snap.pid ? t('evt.startedDetail', { pid: snap.pid }) : '', tone: 'ok', time };
  }
  if (/stop|shutdown|exit/.test(rule)) {
    return { title: t('evt.stopped'), detail: '', tone: 'info', time };
  }
  if (/config|reload/.test(rule)) {
    return { title: t('evt.configReloaded'), detail: '', tone: 'ok', time };
  }
  if (/request|traffic|qps/.test(rule)) {
    const d = snap.requests_delta;
    return {
      title: t('evt.trafficChange'),
      detail: typeof d === 'number' ? t('evt.trafficChangeDetail', { n: d }) : '',
      tone: 'info', time,
    };
  }

  // 后端已给中文说明时直接用
  if (/[\u4e00-\u9fa5]/.test(msg)) {
    return { title: msg.split('\n')[0].slice(0, 80), detail: '', tone, time };
  }

  // 无法归类的技术文本：不展示原文，给中性描述
  return { title: t('evt.generic'), detail: '', tone, time };
}

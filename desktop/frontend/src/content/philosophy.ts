/**
 * 哲学语料：用于「不期而遇」的瞬间——开屏片刻、服务启停、特殊时刻、深夜守望与页脚眉批。
 * 属于内容而非界面文案，独立存放并中英一一对应，不走 i18n 字典。
 */

export interface Quote {
  text: string;
  author: string;
}

export interface TimeMood {
  key: 'dawn' | 'day' | 'dusk' | 'night';
  greeting: string;
}

const QUOTES_ZH: Quote[] = [
  { text: '人不能两次踏入同一条河流，但每一次请求都值得一次郑重的握手。', author: '赫拉克利特 · 变奏' },
  { text: '秩序不是没有变化，而是变化在恰当的边界里从容发生。', author: '关于系统' },
  { text: '最好的守护往往不被看见，就像空气，就像静默运转的端口。', author: '值班室笔记' },
  { text: '万物皆有裂隙，那是光照进来的地方；日志亦有留白，那是安宁所在。', author: '莱昂纳德·科恩 · 变奏' },
  { text: '慢下来并非停滞，而是让意义追上速度。', author: '关于时间' },
  { text: '简单是复杂的终点，而非起点。', author: '奥利弗·霍姆斯' },
  { text: '信任从来不是凭空而来，它由一环扣一环的证明悄然建立。', author: '关于证书链' },
  { text: '我们塑造工具，而后工具也在静默中塑造我们。', author: '马歇尔·麦克卢汉' },
  { text: '在无序的噪音里辨认出一条清晰的信号，本身就是一种诗意。', author: '关于观测' },
  { text: '静止不是终结，是下一次出发前的深呼吸。', author: '关于留白' },
  { text: '真正的优雅，是把复杂留给自己，把从容交予世界。', author: '关于设计' },
  { text: '知止而后有定，定而后能静，静而后能安。', author: '《大学》' },
  { text: '每一次重连，都是世界愿意再给彼此一次机会。', author: '关于网络' },
  { text: '不要因为走得太快，而忘了为什么点亮第一盏灯。', author: '致造物者' },
];

const QUOTES_EN: Quote[] = [
  { text: 'No one steps in the same river twice, yet every request deserves a proper handshake.', author: 'After Heraclitus' },
  { text: 'Order is not the absence of change, but change unfolding gracefully within bounds.', author: 'On Systems' },
  { text: 'The finest guardians stay unseen — like air, like a quietly listening port.', author: 'Watchtower Notes' },
  { text: 'There is a crack in everything; that is how the light gets in.', author: 'Leonard Cohen' },
  { text: 'Slowing down is not stopping; it is letting meaning catch up with speed.', author: 'On Time' },
  { text: 'Simplicity on the far side of complexity is the only kind worth keeping.', author: 'Oliver Wendell Holmes' },
  { text: 'Trust is never magic — it is built quietly, link by verified link.', author: 'On Trust Chains' },
  { text: 'We shape our tools, and thereafter our tools silently shape us.', author: 'Marshall McLuhan' },
  { text: 'Finding one clear signal inside an ocean of noise is its own kind of poetry.', author: 'On Observability' },
  { text: 'Stillness is not an ending; it is the breath before the next beginning.', author: 'On Pause' },
  { text: 'True elegance keeps the complexity within and offers calm to the world.', author: 'On Craft' },
  { text: 'Knowing where to rest brings steadiness; steadiness brings quiet.', author: 'The Great Learning' },
  { text: 'Every reconnect is the world offering two distant points another chance.', author: 'On Networks' },
  { text: 'Never move so fast that you forget why you lit the first lamp.', author: 'To the Builder' },
];

const START_WHISPERS_ZH = [
  '端口已苏醒，静候第一次握手。',
  '灯已点亮，秩序开始流动。',
  '引擎就位，余下的交给静默与时间。',
];
const START_WHISPERS_EN = [
  'The port awakens, awaiting its first handshake.',
  'The lamp is lit; order begins to flow.',
  'Standing watch. Let silence and time do the rest.',
];

const STOP_WHISPERS_ZH = [
  '暂且停泊，也是航行的一部分。',
  '喧嚣退去，留白同样是一种状态。',
  '已安然收锚，待下一次启程。',
];
const STOP_WHISPERS_EN = [
  'Dropping anchor is also part of the voyage.',
  'The current settles; stillness is a state of its own.',
  'Resting quietly until the next departure.',
];

const SYMMETRIC_TIMES = new Set([
  '00:00', '01:10', '02:20', '03:30', '04:40', '05:50',
  '10:01', '11:11', '12:21', '13:31', '14:41', '15:51',
  '20:02', '21:12', '22:22', '23:32',
]);

export function quoteForMoment(locale: string, now: Date = new Date()): Quote {
  const list = locale === 'en-US' ? QUOTES_EN : QUOTES_ZH;
  const day = Math.floor(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) / 86400000);
  const slot = Math.floor(now.getHours() / 6);
  return list[Math.abs(day * 3 + slot) % list.length];
}

/** 监控页底部的极淡眉批，与开屏取错开的一条，避免重复。 */
export function colophonOfMoment(locale: string, now: Date = new Date()): Quote {
  const list = locale === 'en-US' ? QUOTES_EN : QUOTES_ZH;
  const day = Math.floor(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) / 86400000);
  const slot = Math.floor(now.getHours() / 6);
  return list[Math.abs(day * 3 + slot + 5) % list.length];
}

export function timeMoodOf(locale: string, now: Date = new Date()): TimeMood {
  const h = now.getHours();
  const en = locale === 'en-US';
  if (h >= 5 && h < 11) return { key: 'dawn', greeting: en ? 'Morning Light' : '晨光初启' };
  if (h >= 11 && h < 17) return { key: 'day', greeting: en ? 'Midday Clarity' : '白昼清明' };
  if (h >= 17 && h < 22) return { key: 'dusk', greeting: en ? 'Evening Calm' : '暮色渐沉' };
  return { key: 'night', greeting: en ? 'Still of Night' : '深夜静守' };
}

export function transitionWhisper(locale: string, running: boolean, seed: number): string {
  const list = locale === 'en-US'
    ? (running ? START_WHISPERS_EN : STOP_WHISPERS_EN)
    : (running ? START_WHISPERS_ZH : STOP_WHISPERS_ZH);
  return list[Math.abs(seed) % list.length];
}

/** 撞见零点、对称时刻（11:11 / 22:22 / 12:21…）或整点时，返回一瞬微语。 */
export function specialClockWhisper(locale: string, now: Date): { key: string; text: string } | null {
  const hh = String(now.getHours()).padStart(2, '0');
  const mm = String(now.getMinutes()).padStart(2, '0');
  const hm = `${hh}:${mm}`;
  const en = locale === 'en-US';

  if (hm === '00:00') {
    return {
      key: `midnight-${now.getDate()}`,
      text: en ? 'Midnight turns the page — a quiet new day begins.' : '零点翻页，安静的新一日开始了。',
    };
  }
  if (SYMMETRIC_TIMES.has(hm)) {
    return {
      key: `sym-${hm}`,
      text: en ? `${hm} — time folds into symmetry for a brief minute.` : `${hm} · 此刻时间恰好对称。`,
    };
  }
  if (mm === '00' && now.getSeconds() < 10) {
    return {
      key: `hour-${hh}`,
      text: en ? `${hh}:00 — another hour passes gently.` : `${hh}:00 · 又一小时静静流过。`,
    };
  }
  return null;
}

export function lateNightWhisper(locale: string): string {
  return locale === 'en-US'
    ? 'It is late. The machine will keep watch — you do not have to stay awake.'
    : '夜深了，机器会替你守着，人不必一直醒着。';
}

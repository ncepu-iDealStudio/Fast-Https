/**
 * 每日一句：只放在空状态这类低信息密度位置。
 *
 * 同一天内固定，避免刷新时文字变化分散注意力；按本地日期取模轮换。
 * 这里不是界面文案而是内容，所以放在独立文件，不走 i18n 字典；
 * 中英数组一一对应，切换语言时索引不变。
 */
const ZH = [
  '安静不代表停摆，很多系统就是这样工作的。',
  '没有消息，往往就是最好的消息。',
  '一切正常的时候，日志总是显得有点无聊。',
  '今天什么都没有发生，这本身就是一件好事。',
  '机器在按部就班，你也可以稍微慢一点。',
  '稳定从来不是运气，是昨天做对了很多事。',
  '空白也是一条记录：此刻，一切如常。',
  '没有异常，是运维人员最朴素的成就感。',
  '别看它空，它在替你挡掉很多麻烦。',
  '时间在这里走得很轻，只是有人在看着它。',
];

const EN = [
  "Quiet doesn't mean idle — most systems do their best work silently.",
  'No news is usually good news.',
  "When everything works, logs get a little boring. That's a compliment.",
  "Nothing happened today. That's the goal, not a failure.",
  "The machine keeps its pace. You don't have to match it.",
  "Stability is never luck — it's yesterday's good decisions.",
  'An empty page is still a record: all is well, for now.',
  'No alerts is the quiet pride of anyone who runs systems.',
  "It looks empty, but it's been busy keeping trouble away.",
  'Time passes gently here. Someone is watching over it.',
];

export function quipOfDay(locale: string, now: Date = new Date()): string {
  const list = locale === 'en-US' ? EN : ZH;
  const day = Math.floor(
    Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) / 86400000,
  );
  return list[Math.abs(day) % list.length];
}

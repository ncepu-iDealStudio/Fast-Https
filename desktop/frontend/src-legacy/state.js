// 跨模块共享的可变状态：各页面模块按需读写，
// 入口 main.js 只负责初始化时序与定时器。
export const state = {
  currentLog: 'access',

  overviewBusy: false,
  detailsBusy: false,
  lastOverview: null,
  lastRunning: null,
  errorPushed: false,
  firstOverview: true,
  events: [],

  configDirty: false,
  configLoaded: false,
  configInitial: {},

  siteMode: 'add',
  // siteEditingId 是站点详情弹窗当前编辑的站点名（model.Site.id），
  // 编辑提交时回填到 SiteInput.id，对应 site update <name>。
  siteEditingId: '',
  siteEditingName: '',

  // logCursor 是日志页「加载更早」的游标：上一页 page.nextUntil
  // （本页最早一条的时间），下一页作为 QueryLogs.cursor 传入。
  logCursor: '',

  trustGuideData: null,
};

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
  siteEditingSource: '',

  trustGuideData: null,
};

# 前端回归测试

## 当前状态

vanilla 前端时代的 `run.mjs`（32 条浏览器断言）已随旧代码一起删除：
它按旧 DOM（`#siteAdvanced`、`[data-nav]`、`.site-more`）与
`/src/ui/loading.js` 定位，与 Vue 3 的 DOM 不兼容，无法继续运行。

功能清单保存在 [REFACTOR-BASELINE.md](../REFACTOR-BASELINE.md)，
逐页迁移时按该清单补等价断言。

## 迁移后的等价覆盖（待建）

- `vitest` + `@vue/test-utils`：组件级测试，覆盖各 pane 与弹窗的交互
- `playwright`：关键路径截图与端到端（启动、站点增删改、配置保存）
- 每完成一个 pane，在 REFACTOR-BASELINE.md 勾掉对应项并补测试

## 当前可跑的守卫

- `npm run typecheck` — vue-tsc 零错误
- `npm run i18n:check` — 两份字典各 554 键、代码引用全覆盖
- `npm run build` — 上述两项 + Vite 生产构建

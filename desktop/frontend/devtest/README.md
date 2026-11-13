# 前端回归测试

## 当前状态（Vue 3 重构中）

`run.mjs` 是 **vanilla 前端时代**的浏览器回归：32 条断言全部按旧 DOM
（`#siteAdvanced`、`[data-nav]`、`.site-more`、`/src/ui/loading.js`）定位。
Vue 3 前端的 DOM 与交互模型已完全不同，这套脚本**暂时无法运行**，
保留在这里作为「功能清单」的可执行参照（对应 REFACTOR-BASELINE.md）。

旧前端源码在 `src-legacy/`，旧的 `index.html` 在 `index-legacy.html`。
如果要复跑旧断言，把这两者恢复、并把 `devtest/vite.config.mjs` 的
`root` 指回旧入口即可。

## 迁移后的等价覆盖

逐页迁移时，为每个 pane 补 Vitest 组件测试（`@vue/test-utils`）与
Playwright 截图，确保 REFACTOR-BASELINE.md 的每个勾选项都有对应断言。

## 当前可跑的守卫

- `npm run typecheck` — vue-tsc 零错误
- `npm run i18n:check` — 两份字典各 554 键、代码引用全覆盖
- `npm run build` — 上述两项 + Vite 生产构建

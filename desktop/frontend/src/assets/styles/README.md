# 样式组织结构

所有样式集中在这里，**组件 `.vue` 文件里不写 `<style>`**（包括 scoped）。
`src/main.ts` 只 import `assets/styles/index.css` 这一个入口。

```
assets/styles/
├── index.css              总入口：按 components → layout → views 顺序 @import
├── components/            通用组件与设计令牌
│   ├── index.css
│   ├── tokens.css         设计变量（亮/暗双主题）+ 重置 + 字号/间距/圆角收敛
│   ├── fh-button.css      FhButton.vue
│   ├── fh-dialog.css      FhDialog.vue（Reka UI 原语的外观层）
│   ├── fh-field.css       FhField.vue（统一表单行 grid）
│   └── badge.css          状态徽标
├── layout/                窗口骨架与全局排版
│   ├── index.css
│   └── window.css         顶栏、侧边导航、主区
└── views/                 按页面
    ├── index.css
    └── overview.css       概览页卡片
```

## 约定

1. **一个组件/页面一个 CSS 文件**，文件名与组件名对应；新建后必须在所在层的
   `index.css` 补一行 `@import`，不要直接写进业务组件。
2. **类名前缀 `fh-`**（通用组件）或层内命名空间（`.window`、`.card`），
   避免全局样式串味；Dialog 内容由 Reka UI teleport 到 body，全局类名更稳。
3. **只用 `tokens.css` 里的变量**：`--fs-*`、`--sp-*`、`--radius-*`、`--accent-*` 等。
   不再散写 `11px / 12px / 12.5px` 字号或 `6 / 8 / 10 / 14px` 间距。
4. 顺序固定：tokens → components → layout → views，后层可覆盖前层。
5. 旧版 `src/styles/*.css`（base / components / index / layout / pages，共 656 行）
   已删除，其内容按上述三层重新组织，规则一条未丢。

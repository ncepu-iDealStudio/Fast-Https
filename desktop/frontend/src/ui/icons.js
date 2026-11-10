// 统一的线性图标集：24×24 视区、1.7 描边、圆头圆角、跟随文字颜色
// （暗色主题自动适配）。所有图标放在这里一处维护；
// 静态 HTML 里写 <span class="ico" data-ic="globe"></span>，启动时
// initIcons() 替换成 SVG；JS 动态渲染的行直接调 iconSVG(name)。
//
// 画的是形状本身，不引入外部图标字体/依赖；play / stop / 圆点这类
// 实心图形在路径上单独 fill。
export const ICONS = {
  // 概览：实时脉冲线
  pulse: '<path d="M3 12h3.6l2.6-6.2 4 12.4 2.6-6.2H21"/>',
  // 状态矩阵：四宫格
  grid: '<rect x="3.5" y="3.5" width="7" height="7" rx="1.7"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.7"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.7"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.7"/>',
  // 站点：地球（网站）
  globe: '<circle cx="12" cy="12" r="8.6"/><path d="M3.4 12h17.2"/><path d="M12 3.4c2.4 2.4 3.7 5.3 3.7 8.6s-1.3 6.2-3.7 8.6c-2.4-2.4-3.7-5.3-3.7-8.6S9.6 5.8 12 3.4z"/>',
  // 证书：盾牌加对勾
  shield: '<path d="M12 3.2l7 2.7v5.3c0 4.3-2.8 8-7 9.6-4.2-1.6-7-5.3-7-9.6V5.9z"/><path d="M8.7 12.2l2.3 2.3 4.3-4.8"/>',
  // 日志：带行标的清单
  logs: '<path d="M9.5 6h11M9.5 12h11M9.5 18h11"/><circle cx="4.7" cy="6" r="1.25" fill="currentColor" stroke="none"/><circle cx="4.7" cy="12" r="1.25" fill="currentColor" stroke="none"/><circle cx="4.7" cy="18" r="1.25" fill="currentColor" stroke="none"/>',
  // 命令面板：终端
  terminal: '<rect x="3" y="4.5" width="18" height="15" rx="2.2"/><path d="M7.2 9.6l2.8 2.4-2.8 2.4M12.8 15h4.2"/>',
  // 打开配置目录：文件夹
  folder: '<path d="M3.5 6.8a2 2 0 0 1 2-2h3.4l2.1 2.3h7.5a2 2 0 0 1 2 2v8.7a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"/>',
  // 启动：实心三角
  play: '<path d="M8.8 5.7v12.6L18.2 12z" fill="currentColor"/>',
  // 停止：实心方块
  stop: '<rect x="6.6" y="6.6" width="10.8" height="10.8" rx="2" fill="currentColor"/>',
  // 重载：顺时针回环箭头
  reload: '<path d="M19.6 14.6a8.5 8.5 0 1 1-1.9-9.2l3 4.1"/><path d="M20.7 4.9v4.6h-4.6"/>',
  // 站点启停：电源
  power: '<path d="M12 3.2v8"/><path d="M7.4 6.3a7 7 0 1 0 9.2 0"/>',
  // 删除：垃圾桶
  trash: '<path d="M4.5 6.7h15"/><path d="M9.3 6.7V4.8h5.4v1.9"/><path d="M6.6 6.7l.8 11.8a1.8 1.8 0 0 0 1.8 1.7h5.6a1.8 1.8 0 0 0 1.8-1.7l.8-11.8"/><path d="M10.2 10.5v5M13.8 10.5v5"/>',
  // 详情：省略号
  more: '<circle cx="5.5" cy="12" r="1.5" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none"/><circle cx="18.5" cy="12" r="1.5" fill="currentColor" stroke="none"/>',
  // 证书检查：放大镜加对勾
  search: '<circle cx="10.6" cy="10.6" r="6.4"/><path d="M15.4 15.4L20.5 20.5"/><path d="M7.8 10.6l1.9 1.9 3.5-4"/>',
  // 菜单展开尖角（向上）
  chevronUp: '<path d="M6 14.5l6-5.5 6 5.5"/>'
};

// iconSVG 返回完整的 <svg> 标记；cls 默认 .ic（尺寸描边在 CSS 里统一管）。
export function iconSVG(name, cls = 'ic') {
  return '<svg class="' + cls + '" viewBox="0 0 24 24" aria-hidden="true">' + (ICONS[name] || '') + '</svg>';
}

// initIcons 把静态 HTML 里的 <span data-ic="..."> 换成 SVG（类名保留，
// 便于 .glyph / .quick-ico 等继续控制间距与尺寸）。
// 入口在模块初始化前调用一次即可。
export function initIcons(root = document) {
  root.querySelectorAll('span[data-ic]').forEach((el) => {
    const wrap = document.createElement('span');
    wrap.innerHTML = iconSVG(el.getAttribute('data-ic'), 'ic ' + el.className.replace(/\bico\b/g, '').trim());
    el.replaceWith(wrap.firstElementChild);
  });
}

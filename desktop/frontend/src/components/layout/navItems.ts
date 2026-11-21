/** 侧栏导航与顶栏面包屑共用的页面清单（图标 + 字典键）。 */
import type { Component } from 'vue';
import {
  Activity, Globe, Database, ShieldCheck, ScrollText, History, ClipboardList, Settings2,
} from '@lucide/vue';
import type { PaneKey } from '../../stores/app';

export interface NavItem { key: PaneKey; label: string; icon: Component }

export const NAV_ITEMS: NavItem[] = [
  { key: 'monitor', label: 'monitor.center', icon: Activity },
  { key: 'sites', label: 'nav.sites', icon: Globe },
  { key: 'cache', label: 'nav.cache', icon: Database },
  { key: 'certs', label: 'nav.certs', icon: ShieldCheck },
  { key: 'logs', label: 'nav.logs', icon: ScrollText },
  { key: 'operations', label: 'nav.operations', icon: History },
  { key: 'audit', label: 'nav.audit', icon: ClipboardList },
];

/** 不在侧栏里、但能被打开的页面（设置由底部账户菜单进入）。 */
const EXTRA: NavItem[] = [
  { key: 'settings', label: 'settings.configTitle', icon: Settings2 },
];

export function navItemOf(key: PaneKey): NavItem {
  return NAV_ITEMS.find((i) => i.key === key) ?? EXTRA.find((i) => i.key === key) ?? NAV_ITEMS[0];
}

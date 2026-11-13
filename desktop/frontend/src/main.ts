// Vue 3 前端唯一入口。样式只从 assets/styles/index.css 引入，
// 组件内不再写 <style>——全部按 components / layout / views 三层组织。
import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import { i18n } from './i18n';
import './assets/styles/index.css';

createApp(App)
  .use(createPinia())
  .use(i18n)
  .mount('#app');

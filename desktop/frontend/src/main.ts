// Vue 3 前端唯一入口。样式只从 assets/styles/index.css 引入，
// 组件内不写 <style>——全部按 components / layout / views 三层组织。
import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import { i18n } from './i18n';
import './assets/styles/index.css';

const app = createApp(App);

// 全局兜底：任何未捕获错误都记到控制台并摘掉首屏骨架，
// 避免界面卡在「正在加载界面…」而看不出原因。
app.config.errorHandler = (err) => {
  console.error('[fast-https] 渲染出错：', err);
  document.getElementById('boot')?.remove();
};

app.use(createPinia())
  .use(i18n)
  .mount('#app');

// 挂载完成后再移除首屏骨架，两者不会同时可见。
document.getElementById('boot')?.remove();

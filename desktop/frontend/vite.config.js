import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url';

// Vue 3 前端（src/）配置。Wails 从 URL "/" 加载，产物是 dist/index.html。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  // 首次 dev 启动时 Vite 会「先报 ready、再后台预构建依赖」，Webview 在
  // 依赖没打包完就请求模块，会拿到不完整的模块图 —— 表现为界面只画出
  // 侧边栏、其余区域空白。把运行时依赖全部显式列进 include，Vite 会在
  // 报告 ready 之前完成预构建，消除这个竞态。
  optimizeDeps: {
    include: [
      'vue',
      'pinia',
      'vue-i18n',
      '@vueuse/core',
      '@lucide/vue',
      'reka-ui',
    ],
  },
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
});

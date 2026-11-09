// 点击测试台的 Vite 配置：以真实前端为根，把 bindings 导入劫持到
// mock-service，这样页面在纯浏览器里跑起来，不依赖 Wails 运行时。
import { defineConfig } from 'vite';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  root: path.resolve(here, '..'),
  clearScreen: false,
  plugins: [{
    name: 'mock-bindings',
    enforce: 'pre',
    resolveId(source) {
      if (source.includes('bindings/fast-https-desktop')) {
        return path.resolve(here, 'mock-service.js');
      }
      return null;
    }
  }],
  server: { port: 9247, strictPort: true }
});

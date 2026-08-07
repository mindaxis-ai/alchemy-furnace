import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'

/**
 * Vite 配置文件
 * - 使用 @vitejs/plugin-react 支持 React 18
 * - 配置路径别名 @ 指向 src 目录
 * - 代理 API 请求到后端服务
 */
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 3000,
    host: true,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true, // 支持 WebSocket 流式对话代理
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
})

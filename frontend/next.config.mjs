import createNextIntlPlugin from 'next-intl/plugin'

const withNextIntl = createNextIntlPlugin('./i18n.ts')

const isDev = process.env.NODE_ENV !== 'production'

const nextConfig = {
  // 生产静态导出：产物 out/ 由 nginx 服务（沿用既有部署形态）；
  // 开发环境不导出，动态路由按需渲染（export 会强制校验 generateStaticParams）
  ...(isDev ? {} : { output: 'export' }),
  images: { unoptimized: true },
  // 开发环境将 REST API 代理到 Go 后端；WebSocket 走 NEXT_PUBLIC_WS_BASE_URL 直连
  ...(isDev
    ? {
        rewrites: async () => [
          {
            source: '/api/:path*',
            destination: 'http://localhost:8080/api/:path*',
          },
        ],
        // 融合/合成长调用可达 180s+,Next.js dev proxy 默认 30s 超时会导致 ECONNRESET
        // 显式放宽到 5 分钟,前端保持耐心,Go/Python 端超时仍由各自 client 控制
        experimental: {
          proxyTimeout: 300_000,
        },
      }
    : {}),
}

export default withNextIntl(nextConfig)

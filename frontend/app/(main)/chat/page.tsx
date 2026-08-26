/**
 * 论道页。
 *
 * 桌面端为 Next output:export 静态站点，会话 id 通过查询参数 ?session=<UUID>
 * 传递（动态路由 /chat/[sessionId] 只预渲染占位 _，无法恢复真实 UUID）。
 * useSearchParams() 必须包在 Suspense 中才能静态导出。
 */
import { Suspense } from 'react'
import { ChatPageClient } from './chat-page-client'

export default function ChatPage() {
  return (
    <Suspense fallback={<main aria-busy="true" className="min-h-screen" />}>
      <ChatPageClient />
    </Suspense>
  )
}

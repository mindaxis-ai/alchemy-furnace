'use client'

/**
 * 论道页客户端适配层 —— 唯一读取 useSearchParams() 的地方。
 *
 * 桌面端是 Next output:export 静态站点，动态路由无法携带真实 UUID，
 * 会话 id 只能来自查询参数 ?session=<UUID>。这里负责把它解析并校验后
 * 透传给 ChatView；page.tsx 仅提供静态导出所需的 Suspense 边界。
 */
import { useSearchParams } from 'next/navigation'
import { ChatView } from './chat-view'
import { parseChatSessionId } from '@/lib/chat-route'

export function ChatPageClient() {
  const searchParams = useSearchParams()
  return <ChatView sessionId={parseChatSessionId(searchParams)} />
}

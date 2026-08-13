'use client'

import type { ReactNode } from 'react'
import { PillProvider } from '@/contexts/PillContext'
import { AgentProvider } from '@/contexts/AgentContext'
import { ChatProvider } from '@/contexts/ChatContext'
import { UserProvider } from '@/contexts/UserContext'
import { HashRedirect } from '@/components/hash-redirect'

/**
 * 客户端 Provider 组合：金丹 / 道人 / 对话 / 用户档案
 * 附带旧 HashRouter 链接兼容重定向
 */
export function Providers({ children }: { children: ReactNode }) {
  return (
    <UserProvider>
      <PillProvider>
        <AgentProvider>
          <ChatProvider>
            <HashRedirect />
            {children}
          </ChatProvider>
        </AgentProvider>
      </PillProvider>
    </UserProvider>
  )
}

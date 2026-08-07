/**
 * 应用根组件 - 路由配置
 * 使用 HashRouter 兼容静态部署
 * 包含所有页面路由和 Context Provider 包裹
 */
import { HashRouter, Routes, Route } from 'react-router-dom'
import { PillProvider } from '@/contexts/PillContext'
import { AgentProvider } from '@/contexts/AgentContext'
import { ChatProvider } from '@/contexts/ChatContext'

// 页面组件
import Home from '@/pages/Home'
import Pills from '@/pages/Pills'
import PillDetail from '@/pages/PillDetail'
import Agents from '@/pages/Agents'
import AgentDetail from '@/pages/AgentDetail'
import Chat from '@/pages/Chat'
import Settings from '@/pages/Settings'

/**
 * 应用根组件
 * 包裹所有 Context Provider 和路由配置
 */
function App() {
  return (
    <HashRouter>
      <PillProvider>
        <AgentProvider>
          <ChatProvider>
            <Routes>
              {/* 首页 - 炼丹炉主殿 */}
              <Route path="/" element={<Home />} />

              {/* 金丹阁 - 语言模式金丹管理 */}
              <Route path="/pills" element={<Pills />} />
              <Route path="/pills/:id" element={<PillDetail />} />

              {/* 道人府 - Agent 管理 */}
              <Route path="/agents" element={<Agents />} />
              <Route path="/agents/:id" element={<AgentDetail />} />

              {/* 论道 - 对话 */}
              <Route path="/chat" element={<Chat />} />
              <Route path="/chat/:sessionId" element={<Chat />} />

              {/* 设置 */}
              <Route path="/settings" element={<Settings />} />
            </Routes>
          </ChatProvider>
        </AgentProvider>
      </PillProvider>
    </HashRouter>
  )
}

export default App

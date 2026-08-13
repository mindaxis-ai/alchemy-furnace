'use client'

/**
 * 论道视图 - 对话大厅
 * 左侧: 会话列表（可折叠，H5 默认折叠为底部 sheet）
 * 右侧: 聊天界面
 * 选择道人后开始对话（标准 SSE 流式输出，无 RAG 引用来源）
 * SSE：fetch POST + ReadableStream；停止 = AbortController 中断连接
 */
import { useState, useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'
import {
  MessageSquare,
  Plus,
  Loader2,
  ChevronLeft,
  Bot,
  Send,
  X,
  Menu,
  Sparkles,
  Users,
  Square,
  AlertCircle,
} from 'lucide-react'
import { useChat } from '@/contexts/ChatContext'
import { useAgent } from '@/contexts/AgentContext'
import { ChatMessage } from '@/components/chat-message'
import { TopTabs } from '@/components/interaction/top-tabs'
import { OnboardingCard } from '@/components/onboarding-card'
import { listProviders } from '@/services/modelService'

export function ChatView({ sessionId }: { sessionId?: string }) {
  const router = useRouter()

  const { state: chatState, dispatch, fetchSessions, loadMessages, streamMessage, createSession, stopStream } = useChat()
  const { state: agentState, fetchAgents } = useAgent()

  const [input, setInput] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [showAgentSelect, setShowAgentSelect] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const [providersEmpty, setProvidersEmpty] = useState<boolean | null>(null)

  const currentSession = chatState.currentSession
  const messages = chatState.messages
  const sessions = chatState.sessions
  const agents = agentState.agents

  // 初始化加载
  useEffect(() => {
    fetchSessions()
    fetchAgents()
  }, [fetchSessions, fetchAgents])

  // 首启引导: 拉供应商列表,空 → 显示 OnboardingCard
  // null = 加载中(防闪烁),true = 空, false = 已有
  useEffect(() => {
    let cancelled = false
    listProviders({ page: 1, page_size: 1 })
      .then((res) => { if (!cancelled) setProvidersEmpty((res?.list?.length ?? 0) === 0) })
      .catch(() => { if (!cancelled) setProvidersEmpty(false) })
    return () => { cancelled = true }
  }, [])

  // 根据 URL 参数加载会话
  useEffect(() => {
    if (sessionId) {
      loadMessages(sessionId)
    } else {
      dispatch({ type: 'CLEAR_CURRENT' })
    }
  }, [sessionId, loadMessages, dispatch])

  // 自动滚动到底部
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, chatState.streaming])

  /** 发送消息 */
  const handleSend = async () => {
    if (!input.trim() || !currentSession || chatState.streaming) return
    const content = input.trim()
    setInput('')
    await streamMessage(currentSession.id, content)
  }

  /** 创建会话并跳转 */
  const handleCreateSession = async (agentId: string) => {
    const agent = agents.find(a => a.id === agentId)
    const session = await createSession(agentId, `与${agent?.name || '未知道人'}的论道`)
    setShowAgentSelect(false)
    if (session) router.push(`/chat/${session.id}`)
  }

  /** 选择会话 */
  const handleSelectSession = (sid: string) => {
    router.push(`/chat/${sid}`)
    setSidebarOpen(false)
  }

  /** 获取道人称谓 */
  const getAgentName = (agentId: string) => {
    return agents.find(a => a.id === agentId)?.name || `道人 #${agentId}`
  }

  /** 获取头像首字 */
  const getAgentInitial = (agentId: string) => {
    const name = getAgentName(agentId)
    return name.charAt(0)
  }

  // 如果没有选择会话: 先看是否需要首启引导(无供应商)
  if (!currentSession) {
    if (providersEmpty === true) {
      return (
        <div className="mx-auto max-w-6xl px-4 sm:px-6">
          <div className="flex flex-col items-center justify-center h-[60vh]">
            <OnboardingCard />
          </div>
        </div>
      )
    }
    return (
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="flex flex-col items-center justify-center h-[60vh] text-center">
          <MessageSquare className="w-16 h-16 text-sage/50 mb-4" />
          <h2 className="text-xl font-serif font-bold text-foreground mb-2">
            论道
          </h2>
          <p className="text-sm text-muted-foreground mb-6 max-w-sm">
            选择一位道人，开始你的论道之旅。道人将以基础性格融合已服用金丹的丹性与你对谈。
          </p>

          <button
            onClick={() => setShowAgentSelect(true)}
            className="dao-btn-primary"
          >
            <Plus className="w-4 h-4" />
            新建对话
          </button>

          {/* 已有会话列表入口 */}
          {sessions.length > 0 && (
            <div className="mt-8 w-full max-w-md">
              <p className="text-xs text-sage/70 mb-3">或选择已有对话</p>
              <div className="space-y-2">
                {sessions.slice(0, 5).map(session => (
                  <button
                    key={session.id}
                    onClick={() => handleSelectSession(session.id)}
                    className="dao-card w-full p-3 flex items-center gap-3 text-left"
                  >
                    <div className="w-8 h-8 rounded-full bg-sage/15 flex items-center justify-center flex-shrink-0">
                      <Bot className="w-4 h-4 text-sage" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm text-foreground truncate">{session.title}</p>
                      <p className="text-[10px] text-muted-foreground">{getAgentName(session.agent_id)}</p>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* 选择道人弹窗 */}
        {showAgentSelect && (
          <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
            <div className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300 max-h-[80vh] overflow-y-auto">
              <div className="flex items-center justify-between mb-5">
                <div className="flex items-center gap-2">
                  <Users className="w-5 h-5 text-sage" />
                  <h2 className="text-lg font-serif font-bold text-gold">选择道人</h2>
                </div>
                <button
                  aria-label="关闭弹窗"
                  onClick={() => setShowAgentSelect(false)}
                  className="p-1.5 rounded-lg hover:bg-secondary text-muted-foreground hover:text-foreground transition-colors"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>

              <div className="space-y-2">
                {agents.filter(a => a.status === 'active').map(agent => (
                  <button
                    key={agent.id}
                    onClick={() => handleCreateSession(agent.id)}
                    className="w-full flex items-center gap-3 p-3 rounded-lg bg-muted border border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left"
                  >
                    <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold flex-shrink-0">
                      {agent.name.charAt(0)}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground">{agent.name}</p>
                      <p className="text-xs text-muted-foreground truncate">{agent.model_name}</p>
                    </div>
                    <Sparkles className="w-4 h-4 text-gold/50" />
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="mx-auto flex h-[calc(100vh-4rem)] max-w-6xl px-4 sm:px-6 relative">
      {/* ========== 会话列表侧边栏 ========== */}
      {/* 桌面端侧边栏 */}
      <div className={`
        hidden md:block
        ${sidebarOpen ? 'w-72' : 'w-0'}
        transition-all duration-300 overflow-hidden
        border-r border-border/70 bg-muted
      `}>
        <div className="w-72 h-full flex flex-col">
          {/* 侧边栏头部 */}
          <div className="flex items-center justify-between p-3 border-b border-border/70">
            <span className="text-sm font-medium text-gold">会话列表</span>
            <button
              aria-label="新建会话"
              onClick={() => setShowAgentSelect(true)}
              className="p-1.5 rounded hover:bg-gold/10 text-gold transition-colors"
            >
              <Plus className="w-4 h-4" />
            </button>
          </div>

          {/* 会话列表 */}
          <div className="flex-1 overflow-y-auto p-2 space-y-1">
            {sessions.map(session => (
              <button
                key={session.id}
                onClick={() => handleSelectSession(session.id)}
                className={`
                  w-full flex items-center gap-2 p-2.5 rounded-lg text-left transition-all
                  ${currentSession?.id === session.id
                    ? 'bg-gold/10 border border-gold/20'
                    : 'hover:bg-secondary/70 border border-transparent'
                  }
                `}
              >
                <div className="w-7 h-7 rounded-full bg-sage/15 flex items-center justify-center flex-shrink-0">
                  <Bot className="w-3.5 h-3.5 text-sage" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className={`text-xs truncate ${currentSession?.id === session.id ? 'text-gold' : 'text-foreground'}`}>
                    {session.title}
                  </p>
                  <p className="text-[10px] text-muted-foreground">{getAgentName(session.agent_id)}</p>
                </div>
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* H5 底部 Sheet */}
      {sidebarOpen && (
        <div className="md:hidden fixed inset-0 z-40 bg-black/50" onClick={() => setSidebarOpen(false)}>
          <div
            className="absolute bottom-0 left-0 right-0 bg-card rounded-t-2xl max-h-[70vh] animate-in fade-in duration-300"
            onClick={e => e.stopPropagation()}
          >
            <div className="flex items-center justify-between p-4 border-b border-border/70">
              <span className="text-sm font-medium text-gold">会话列表</span>
              <div className="flex items-center gap-2">
                <button
                  aria-label="新建会话"
                  onClick={() => { setSidebarOpen(false); setShowAgentSelect(true) }}
                  className="p-1.5 rounded hover:bg-gold/10 text-gold"
                >
                  <Plus className="w-4 h-4" />
                </button>
                <button
                  aria-label="关闭"
                  onClick={() => setSidebarOpen(false)}
                  className="p-1.5 rounded hover:bg-secondary text-muted-foreground"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            </div>
            <div className="overflow-y-auto p-2 space-y-1 max-h-[50vh]">
              {sessions.map(session => (
                <button
                  key={session.id}
                  onClick={() => handleSelectSession(session.id)}
                  className={`
                    w-full flex items-center gap-3 p-3 rounded-lg text-left transition-all
                    ${currentSession?.id === session.id ? 'bg-gold/10' : 'hover:bg-secondary/70'}
                  `}
                >
                  <div className="w-8 h-8 rounded-full bg-sage/15 flex items-center justify-center flex-shrink-0">
                    <Bot className="w-4 h-4 text-sage" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-foreground truncate">{session.title}</p>
                    <p className="text-[10px] text-muted-foreground">{getAgentName(session.agent_id)}</p>
                  </div>
                </button>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* ========== 聊天主区域 ========== */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* 聊天头部 */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-border/70 bg-card/80">
          {/* 桌面端侧边栏切换按钮 */}
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="hidden md:block p-1.5 rounded hover:bg-secondary text-muted-foreground hover:text-gold transition-colors"
            aria-label={sidebarOpen ? '收起会话列表' : '展开会话列表'}
          >
            {sidebarOpen ? <ChevronLeft className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
          </button>

          {/* 移动端标签切换（黑神话式）：论道 / 旧录 */}
          <TopTabs
            tabs={[
              { key: 'chat', label: '论道' },
              { key: 'sessions', label: '旧录' },
            ]}
            activeKey={sidebarOpen ? 'sessions' : 'chat'}
            onChange={key => setSidebarOpen(key === 'sessions')}
            className="border-b-0 md:hidden"
          />

          {/* 道人信息 */}
          <div className="w-8 h-8 rounded-full bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold text-sm flex-shrink-0">
            {getAgentInitial(currentSession.agent_id)}
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-foreground truncate">
              {currentSession.title}
            </p>
            <p className="text-[10px] text-muted-foreground">{getAgentName(currentSession.agent_id)}</p>
          </div>

          {/* 新对话按钮 */}
          <button
            onClick={() => setShowAgentSelect(true)}
            className="p-1.5 rounded hover:bg-gold/10 text-gold transition-colors"
            title="新建对话"
          >
            <Plus className="w-5 h-5" />
          </button>
        </div>

        {/* 消息列表 */}
        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4">
          {messages.length === 0 && (
            <div className="flex flex-col items-center justify-center h-full text-center">
              <Sparkles className="w-10 h-10 text-sage/50 mb-3" />
              <p className="text-sm text-muted-foreground">发送消息开始论道</p>
              <p className="text-xs text-sage/70 mt-1">道人将以金丹化性后的性情为你作答</p>
            </div>
          )}

          {messages.map(message => (
            message.is_error ? (
              /* 服务端错误：内联错误气泡 */
              <div key={message.id} className="flex justify-center animate-in fade-in duration-300">
                <div className="flex items-center gap-2 max-w-[85%] md:max-w-[70%] px-4 py-2.5 rounded-2xl bg-primary/10 border border-primary/30 text-primary">
                  <AlertCircle className="w-4 h-4 flex-shrink-0" />
                  <p className="text-xs leading-relaxed">{message.content}</p>
                </div>
              </div>
            ) : (
              <ChatMessage
                key={message.id}
                message={message}
                streaming={chatState.streaming && message.role === 'assistant' && message.id === '-1'}
              />
            )
          ))}

          {/* 流式输出中指示器 */}
          {chatState.streaming && messages[messages.length - 1]?.role === 'user' && (
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded-full bg-gold/20 text-gold border border-gold/30 flex items-center justify-center flex-shrink-0">
                <Bot className="w-5 h-5" />
              </div>
              <div className="bg-card border border-gold/30 rounded-2xl rounded-tl-sm px-4 py-3 animate-pulse">
                <div className="flex items-center gap-1.5 text-xs text-gold/70">
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  <span>道人思考中...</span>
                </div>
              </div>
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        {/* 输入框 */}
        <div className="px-4 py-3 border-t border-border/70 bg-card/80">
          <div className="flex items-end gap-2">
            <textarea
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  handleSend()
                }
              }}
              placeholder="向道人请教..."
              className="dao-input resize-none min-h-[44px] max-h-[120px] py-2.5"
              rows={1}
            />
            <button
              aria-label={chatState.streaming ? '停止输出' : '发送'}
              onClick={chatState.streaming ? stopStream : handleSend}
              disabled={!chatState.streaming && !input.trim()}
              className="dao-btn-primary px-3 py-2.5 flex-shrink-0 disabled:opacity-40"
              title={chatState.streaming ? '停止输出' : '发送'}
            >
              {chatState.streaming ? (
                <Square className="w-5 h-5" />
              ) : (
                <Send className="w-5 h-5" />
              )}
            </button>
          </div>
          <p className="text-[10px] text-sage/70 mt-1.5 text-center">
            Enter 发送 · Shift+Enter 换行
          </p>
        </div>
      </div>

      {/* 选择道人弹窗 */}
      {showAgentSelect && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300 max-h-[80vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-5">
              <div className="flex items-center gap-2">
                <Users className="w-5 h-5 text-sage" />
                <h2 className="text-lg font-serif font-bold text-gold">选择道人</h2>
              </div>
              <button
                aria-label="关闭弹窗"
                onClick={() => setShowAgentSelect(false)}
                className="p-1.5 rounded-lg hover:bg-secondary text-muted-foreground hover:text-foreground transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-2">
              {agents.filter(a => a.status === 'active').map(agent => (
                <button
                  key={agent.id}
                  onClick={() => handleCreateSession(agent.id)}
                  className="w-full flex items-center gap-3 p-3 rounded-lg bg-muted border border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left"
                >
                  <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold flex-shrink-0">
                    {agent.name.charAt(0)}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground">{agent.name}</p>
                    <p className="text-xs text-muted-foreground truncate">{agent.model_name}</p>
                  </div>
                  <Sparkles className="w-4 h-4 text-gold/50" />
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

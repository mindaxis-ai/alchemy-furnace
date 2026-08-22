'use client'

/**
 * 论道视图 - 对话大厅
 * 左侧: 会话列表（可折叠，H5 默认折叠为底部 sheet）
 * 右侧: 聊天界面
 *
 * 模式选择（chat UX polish 1）:
 *   - 「对谈」: 1v1 单聊模式,选 1 位道人
 *   - 「围炉论道」: 群聊模式,选 ≥2 位道人
 *   入口: + 新建对话 → 弹窗顶部 SegmentedControl 切换
 *
 * @ 提及补全（chat UX polish 3，飞书式）:
 *   - 监听 textarea onChange/onKeyDown
 *   - 检测光标前最近一个 @ 字符,浮层显示匹配的道人
 *   - 上下箭头 + Enter 选中,Escape 取消
 *
 * SSE：fetch POST + ReadableStream；停止 = AbortController 中断连接
 */
import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useTranslations } from 'next-intl'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import {
  Plus,
  Loader2,
  ChevronLeft,
  ChevronDown,
  Bot,
  Send,
  X,
  Menu,
  Sparkles,
  Users,
  Square,
  AlertCircle,
  User,
} from 'lucide-react'
import { useChat } from '@/contexts/ChatContext'
import { useAgent } from '@/contexts/AgentContext'
import { ChatMessage } from '@/components/chat-message'
import { GroupMembersPanel } from '@/components/group-members-panel'
import { TopTabs } from '@/components/interaction/top-tabs'
import { MentionSuggest } from '@/components/mention-suggest'
import { OnboardingCard } from '@/components/onboarding-card'
import { ActionFeedback } from '@/components/interaction/action-feedback'
import { useChatLaunchFlow, type LaunchState } from '@/hooks/use-chat-launch-flow'
import { listProviders } from '@/services/modelService'
import type { Agent } from '@/services/types'

type ChatMode = 'single' | 'group'
type ProviderReadiness =
  | { status: 'loading' }
  | { status: 'ready'; empty: boolean }
  | { status: 'error'; message: string }

async function fetchProviderReadiness(): Promise<ProviderReadiness> {
  try {
    const res = await listProviders({ page: 1, page_size: 1 })
    return { status: 'ready', empty: (res?.list?.length ?? 0) === 0 }
  } catch (error) {
    return {
      status: 'error',
      message: error instanceof Error ? error.message : '',
    }
  }
}

export function ChatView({ sessionId }: { sessionId?: string }) {
  const router = useRouter()
  const t = useTranslations('chatView')
  const launchFlow = useChatLaunchFlow()

  const { state: chatState, dispatch, fetchSessions, loadMessages, loadOlderMessages, clearCurrent, streamMessage, renameSession, stopStream } = useChat()
  const { state: agentState, fetchAgents } = useAgent()

  const [input, setInput] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [showAgentSelect, setShowAgentSelect] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesScrollRef = useRef<HTMLDivElement>(null)
  const providerRequestRef = useRef(0)
  const recoveryInFlightRef = useRef<Set<string>>(new Set())
  /** 是否粘底(用户当前在底部附近)。ref 而非 state:scroll 事件高频触发,避免触发 re-render */
  const stickyToBottomRef = useRef(true)
  /** 「回到底部」浮按钮显示状态 */
  const [showJumpToBottom, setShowJumpToBottom] = useState(false)
  /** 离底多少 px 算粘底(给用户一点容差,避免边界值抖动) */
  const STICKY_THRESHOLD_PX = 80
  /** 供应商列表是否为空(用于显示首启引导) */
  const [providerReadiness, setProviderReadiness] = useState<ProviderReadiness>({ status: 'loading' })

  const currentSession = chatState.currentSession
  const messages = chatState.messages
  const sessions = chatState.sessions
  const agents = agentState.agents
  const readOnlyReason = useMemo(() => {
    if (!currentSession) return null
    if (currentSession.type === 'group') {
      const hasInactiveMember = (currentSession.members || []).some(member =>
        member.status === 'inactive' || agents.find(agent => agent.id === member.agent_id)?.status === 'inactive'
      )
      return hasInactiveMember ? t('readOnly.group') : null
    }
    const status = currentSession.agent_status || agents.find(agent => agent.id === currentSession.agent_id)?.status
    return status === 'inactive' ? t('readOnly.single') : null
  }, [agents, currentSession, t])

  // 初始化加载
  useEffect(() => {
    fetchSessions()
    fetchAgents()
  }, [fetchSessions, fetchAgents])

  // 首启引导: 仅在供应商请求明确成功且列表为空时显示。
  useEffect(() => {
    const requestId = ++providerRequestRef.current
    void fetchProviderReadiness().then(readiness => {
      if (requestId === providerRequestRef.current) setProviderReadiness(readiness)
    })
    return () => { providerRequestRef.current += 1 }
  }, [])

  const retryProviderReadiness = () => {
    const requestId = ++providerRequestRef.current
    setProviderReadiness({ status: 'loading' })
    void fetchProviderReadiness().then(readiness => {
      if (requestId === providerRequestRef.current) setProviderReadiness(readiness)
    })
  }

  // T4 快捷键 ⌘N: desktop-guards 在 window 派发 alchemy:new-session → 此处复用现有 setShowAgentSelect
  useEffect(() => {
    const onNewSession = () => setShowAgentSelect(true)
    window.addEventListener('alchemy:new-session', onNewSession)
    return () => window.removeEventListener('alchemy:new-session', onNewSession)
  }, [])

  // 根据 URL 参数加载会话
  useEffect(() => {
    if (sessionId) {
      loadMessages(sessionId)
    } else {
      clearCurrent()
    }
  }, [sessionId, loadMessages, clearCurrent])

  // 自动滚动到底部 — 仅当用户"粘底"时才跟随,避免抢用户滚轮
  // 流式输出时 messages 每 ~30ms 变一次,如果不判断 stickyToBottom,用户的滚轮会被
  // 持续 scrollIntoView 抢走,导致"滚不上去/抖动"
  useEffect(() => {
    if (!stickyToBottomRef.current) return
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [messages, chatState.streaming])

  // 切换会话:强制粘底 + 滚到底(等消息列表渲染完再滚)
  useEffect(() => {
    if (!currentSession) return
    // 等 messages 渲染完(useEffect 顺序:先 messages effect,后这里)
    const t = setTimeout(() => {
      stickyToBottomRef.current = true
      setShowJumpToBottom(false)
      messagesEndRef.current?.scrollIntoView({ behavior: 'auto', block: 'end' })
    }, 50)
    return () => clearTimeout(t)
  }, [currentSession?.id])

  /** 发送消息 */
  const handleSend = async () => {
    if (!input.trim() || !currentSession || chatState.streaming || readOnlyReason) return
    const content = input.trim()
    setInput('')
    // 发送时启用粘底(用户刚发了消息,理应看到 AI 回复)
    stickyToBottomRef.current = true
    setShowJumpToBottom(false)
    await streamMessage(currentSession.id, content, { interruptedText: t('stream.interrupted') })
  }

  const retryMessage = async (messageIndex: number) => {
    if (!currentSession || chatState.streaming || readOnlyReason) return
    const recoveryMessage = messages[messageIndex]
    if (!recoveryMessage) return
    const recovery = recoveryMessage.recovery || 'none'
    if (recovery === 'none') return
    if (recoveryInFlightRef.current.has(recoveryMessage.id)) return
    recoveryInFlightRef.current.add(recoveryMessage.id)
    dispatch({ type: 'CONSUME_RECOVERY', payload: { messageId: recoveryMessage.id } })
    const persistedRetry = recovery === 'persisted_retry'
    try {
      for (let i = messageIndex - 1; i >= 0; i -= 1) {
        if (messages[i].role === 'user') {
          stickyToBottomRef.current = true
          setShowJumpToBottom(false)
          await streamMessage(currentSession.id, messages[i].content, {
            retry: persistedRetry,
            reuseUserMessage: true,
            interruptedText: t('stream.interrupted'),
            retryBoundaryText: currentSession.type === 'group' && persistedRetry ? t('stream.retryBoundary') : undefined,
          })
          return
        }
      }
    } finally {
      recoveryInFlightRef.current.delete(recoveryMessage.id)
    }
  }

  /** 防止 IME 中文输入中按 Enter 误触发送 */
  const handleSendOnce = () => {
    handleSend()
  }

  const handleSelectSession = (id: string) => {
    setSidebarOpen(false)
    router.push(`/chat/${id}`)
  }

  const handleCreateSession = async (agentId: string) => {
    const launched = await launchFlow.launchSingle(agentId)
    if (launched) {
      setShowAgentSelect(false)
    }
    return launched
  }

  const handleCreateGroupSession = async (memberAgentIds: string[]) => {
    const launched = await launchFlow.launchGroup(memberAgentIds)
    if (launched) {
      setShowAgentSelect(false)
    }
    return launched
  }

  const closeAgentSelect = () => {
    launchFlow.reset()
    setShowAgentSelect(false)
  }

  const getAgentName = (agentId: string) => {
    return agents.find(a => a.id === agentId)?.name || t('mode.selectAgent')
  }
  const getAgentInitial = (agentId: string) => {
    return agents.find(a => a.id === agentId)?.name.charAt(0) || '?'
  }

  // 滚动监听
  const handleMessagesScroll = () => {
    const el = messagesScrollRef.current
    if (!el) return
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    const isSticky = distanceToBottom <= STICKY_THRESHOLD_PX
    stickyToBottomRef.current = isSticky
    setShowJumpToBottom(!isSticky)
  }

  const jumpToBottom = () => {
    stickyToBottomRef.current = true
    setShowJumpToBottom(false)
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }

  const handleLoadOlder = async () => {
    if (!currentSession) return
    const scroll = messagesScrollRef.current
    const previousHeight = scroll?.scrollHeight || 0
    const previousTop = scroll?.scrollTop || 0
    stickyToBottomRef.current = false
    await loadOlderMessages(currentSession.id)
    requestAnimationFrame(() => {
      if (scroll) scroll.scrollTop = previousTop + scroll.scrollHeight - previousHeight
    })
  }

  if (sessionId && (!currentSession || currentSession.id !== sessionId)) {
    const loadState = chatState.sessionLoad
    if (loadState.status === 'not-found') {
      return (
        <SessionLoadState
          title={t('load.sessionNotFoundTitle')}
          message={t('load.sessionNotFoundMessage')}
          retryLabel={t('load.retry')}
          backLabel={t('load.backToLobby')}
          onRetry={() => loadMessages(sessionId)}
        />
      )
    }
    if (loadState.status === 'error') {
      return (
        <SessionLoadState
          title={t('load.sessionErrorTitle')}
          message={loadState.message || t('load.sessionErrorMessage')}
          retryLabel={t('load.retry')}
          backLabel={t('load.backToLobby')}
          onRetry={() => loadMessages(sessionId)}
        />
      )
    }
    return (
      <div className="mx-auto flex h-[calc(100vh-4rem)] max-w-6xl items-center justify-center px-4 sm:px-6">
        <div className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
          <Loader2 className="h-4 w-4 animate-spin text-gold" />
          <span>{t('load.sessionLoading')}</span>
        </div>
      </div>
    )
  }

  // 如果没有选择会话: 先看是否需要首启引导(无供应商)
  if (!currentSession) {
    if (providerReadiness.status === 'ready' && providerReadiness.empty) {
      return (
        <div className="mx-auto max-w-6xl px-4 sm:px-6">
          <div className="flex flex-col items-center justify-center h-[60vh]">
            <OnboardingCard />
          </div>
        </div>
      )
    }
    return (
      <div className="mx-auto flex h-[calc(100vh-4rem)] max-w-6xl px-4 sm:px-6 relative">
        <div className="flex-1 flex flex-col items-center justify-center text-center px-4">
          <Sparkles className="w-12 h-12 text-sage/50 mb-4" />
          <h1 className="text-2xl font-serif font-bold text-gold mb-2">论道</h1>
          <p className="text-sm text-muted-foreground mb-6 max-w-sm">
            选择一位道人，开始你的论道之旅。道人将以基础性格融合已服用金丹的丹性与你对谈。
          </p>

          <button
            onClick={() => setShowAgentSelect(true)}
            className="dao-btn-primary"
          >
            <Plus className="w-4 h-4" />
            {t('newSession')}
          </button>

          <div className="mt-4 flex w-full max-w-md flex-col gap-2 text-left">
            {providerReadiness.status === 'error' && (
              <LoadNotice
                title={t('load.providerError')}
                message={providerReadiness.message || t('load.providerError')}
                retryLabel={t('load.retry')}
                onRetry={retryProviderReadiness}
              />
            )}
            {chatState.sessionsError && (
              <LoadNotice
                title={t('load.sessionsError')}
                message={chatState.sessionsError}
                retryLabel={t('load.retry')}
                onRetry={fetchSessions}
              />
            )}
          </div>

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

        {/* 选择道人弹窗(对谈/围炉论道 模式) */}
        {showAgentSelect && (
          <AgentSelectModal
            agents={agents.filter(a => a.status === 'active')}
            agentError={agentState.error}
            launchState={launchFlow.state}
            onClose={closeAgentSelect}
            onSelectSingle={handleCreateSession}
            onSelectGroup={handleCreateGroupSession}
            onRetry={launchFlow.retry}
            onRetryAgents={fetchAgents}
            onSelectionChange={launchFlow.reset}
          />
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
        <div className="frosted w-72 h-full flex flex-col">
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
                  <X className="w-5 h-5" />
                </button>
              </div>
            </div>
            <div className="overflow-y-auto p-2 space-y-1 max-h-[50vh]">
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

          {/* 会话头:群聊显示群名 + 成员数,单聊显示道人头像 */}
          {currentSession.type === 'group' ? (
            <>
              <div className="w-8 h-8 rounded-full bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold text-sm flex-shrink-0">
                <Users className="w-4 h-4" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground truncate">
                  {currentSession.title}
                </p>
                <p className="text-[10px] text-muted-foreground">
                  围炉论道 · {currentSession.members?.length || 0} 位道人
                </p>
              </div>
            </>
          ) : (
            <>
              <div className="w-8 h-8 rounded-full bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold text-sm flex-shrink-0">
                {getAgentInitial(currentSession.agent_id)}
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground truncate">
                  {currentSession.title}
                </p>
                <p className="text-[10px] text-muted-foreground">{getAgentName(currentSession.agent_id)}</p>
              </div>
            </>
          )}

          {/* 群成员管理(仅群聊) */}
          {currentSession.type === 'group' && (
            <button
              onClick={() => {
                const event = new CustomEvent('open-group-members')
                window.dispatchEvent(event)
              }}
              className="p-1.5 rounded hover:bg-gold/10 text-gold transition-colors"
              title="群成员"
            >
              <Users className="w-5 h-5" />
            </button>
          )}

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
        <div ref={messagesScrollRef} onScroll={handleMessagesScroll} className="flex-1 overflow-y-auto relative px-4 py-5">
          <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
          {(chatState.history.hasOlder || chatState.history.olderError) && (
            <div className="flex flex-col items-center gap-1.5">
              <button
                type="button"
                onClick={() => { void handleLoadOlder() }}
                disabled={chatState.history.loadingOlder}
                className="text-xs font-medium text-gold hover:text-foreground disabled:cursor-wait disabled:opacity-60"
              >
                {chatState.history.loadingOlder ? t('history.loadingOlder') : t('history.loadOlder')}
              </button>
              {chatState.history.olderError && (
                <p role="alert" className="text-xs text-primary">{chatState.history.olderError}</p>
              )}
            </div>
          )}
          {messages.length === 0 && (
            <div className="flex flex-col items-center justify-center h-full text-center">
              <Sparkles className="w-10 h-10 text-sage/50 mb-3" />
              <p className="text-sm text-muted-foreground">发送消息开始论道</p>
              <p className="text-xs text-sage/70 mt-1">道人将以金丹化性后的性情为你作答</p>
            </div>
          )}

          {messages.map((message, messageIndex) => (
            message.is_error ? (
              /* 服务端错误：内联错误气泡 */
              <div key={message.id} className="flex justify-center animate-in fade-in duration-300">
                <div className="flex items-center gap-2 max-w-[85%] md:max-w-[70%] px-4 py-2.5 rounded-2xl bg-primary/10 border border-primary/30 text-primary">
                  <AlertCircle className="w-4 h-4 flex-shrink-0" />
                  <p className="text-xs leading-relaxed">{message.content}</p>
                  {message.retryable && !readOnlyReason && (
                    <button
                      type="button"
                      onClick={() => { void retryMessage(messageIndex) }}
                      className="shrink-0 text-xs font-medium text-gold hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                    >
                      {t('stream.retry')}
                    </button>
                  )}
                </div>
              </div>
            ) : (
              <ChatMessage
                key={message.id}
                message={message}
                streaming={chatState.streaming && message.role === 'assistant' && message.id.startsWith('stream-')}
                members={currentSession.members}
                onRetry={message.incomplete && message.retryable && !readOnlyReason ? () => { void retryMessage(messageIndex) } : undefined}
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

          {/* 「回到底部」浮按钮:用户滚上去后才出现,贴在聊天框右下角(避开消息区) */}
          {showJumpToBottom && (
            <button
              type="button"
              onClick={jumpToBottom}
              aria-label="回到底部"
              title="回到底部"
              className="
                absolute bottom-3 right-4 z-10
                inline-flex items-center gap-1.5
                h-9 px-3.5 rounded-full
                bg-card/95 backdrop-blur
                border border-gold/40 hover:border-gold
                text-gold hover:text-foreground
                shadow-lg shadow-black/20
                text-xs font-medium
                transition-all duration-150
                hover:scale-105 active:scale-95
                animate-in fade-in slide-in-from-bottom-2
              "
            >
              <ChevronDown className="w-4 h-4" />
              <span>回到底部</span>
            </button>
          )}
        </div>

        {/* 输入框 */}
        <ChatInput
          value={input}
          onChange={setInput}
          onSend={handleSendOnce}
          streaming={chatState.streaming}
          onStop={stopStream}
          isGroup={currentSession.type === 'group'}
          members={currentSession.members || []}
          disabled={Boolean(readOnlyReason)}
          disabledReason={readOnlyReason}
        />
      </div>

      {/* 选择道人弹窗(对谈/围炉论道 模式) */}
      {showAgentSelect && (
        <AgentSelectModal
          agents={agents.filter(a => a.status === 'active')}
          agentError={agentState.error}
          launchState={launchFlow.state}
          onClose={closeAgentSelect}
          onSelectSingle={handleCreateSession}
          onSelectGroup={handleCreateGroupSession}
          onRetry={launchFlow.retry}
          onRetryAgents={fetchAgents}
          onSelectionChange={launchFlow.reset}
        />
      )}

      {/* 群成员面板(全局事件触发) */}
      {currentSession.type === 'group' && (
        <GroupMembersPanelSession />
      )}
    </div>
  )
}

function LoadNotice({
  title,
  message,
  retryLabel,
  onRetry,
}: {
  title: string
  message: string
  retryLabel: string
  onRetry: () => void | Promise<void>
}) {
  return (
    <div className="rounded-lg border border-gold/30 bg-card px-3 py-2.5 text-sm" role="alert">
      <p className="font-medium text-foreground">{title}</p>
      <div className="mt-1 flex flex-wrap items-center justify-between gap-2">
        <p className="min-w-0 flex-1 break-words text-xs text-muted-foreground">{message}</p>
        <button type="button" onClick={onRetry} className="dao-btn-ghost shrink-0 text-xs">
          {retryLabel}
        </button>
      </div>
    </div>
  )
}

function SessionLoadState({
  title,
  message,
  retryLabel,
  backLabel,
  onRetry,
}: {
  title: string
  message: string
  retryLabel: string
  backLabel: string
  onRetry: () => void | Promise<void>
}) {
  return (
    <div className="mx-auto flex h-[calc(100vh-4rem)] max-w-6xl items-center justify-center px-4 sm:px-6">
      <div className="w-full max-w-md rounded-xl border border-gold/30 bg-card p-5 shadow-sm" role="alert">
        <div className="flex items-start gap-3">
          <AlertCircle className="mt-0.5 h-5 w-5 shrink-0 text-gold" />
          <div className="min-w-0 flex-1">
            <p className="font-medium text-foreground">{title}</p>
            <p className="mt-1 break-words text-sm leading-relaxed text-muted-foreground">{message}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              <button type="button" onClick={onRetry} className="dao-btn-primary text-xs">
                {retryLabel}
              </button>
              <Link href="/chat" className="dao-btn-ghost text-xs">
                {backLabel}
              </Link>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/* ========== 子组件:选择道人弹窗(对谈 / 围炉论道) ========== */

function AgentSelectModal({
  agents,
  agentError,
  launchState,
  onClose,
  onSelectSingle,
  onSelectGroup,
  onRetry,
  onRetryAgents,
  onSelectionChange,
}: {
  agents: Agent[]
  agentError: string | null
  launchState: LaunchState
  onClose: () => void
  onSelectSingle: (agentId: string) => Promise<boolean>
  onSelectGroup: (agentIds: string[]) => Promise<boolean>
  onRetry: () => Promise<boolean>
  onRetryAgents: () => void | Promise<void>
  onSelectionChange: () => void
}) {
  const t = useTranslations('chatView')
  const [mode, setMode] = useState<ChatMode>('single')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const submitting = launchState.status === 'submitting'

  const toggleSelect = (id: string) => {
    onSelectionChange()
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const handleConfirm = async () => {
    if (mode === 'single') {
      const id = [...selected][0]
      if (id) await onSelectSingle(id)
    } else {
      if (selected.size >= 2) await onSelectGroup([...selected])
    }
  }

  const retry = async () => {
    const launched = await onRetry()
    if (launched) onClose()
  }

  const changeMode = (nextMode: ChatMode) => {
    onSelectionChange()
    setMode(nextMode)
    setSelected(new Set())
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="agent-select-title"
        className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300 max-h-[80vh] flex flex-col"
      >
        {/* 头部 */}
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <Users className="w-5 h-5 text-sage" />
            <h2 id="agent-select-title" className="text-lg font-serif font-bold text-gold">
              {mode === 'single' ? t('mode.selectAgent') : t('mode.selectAgents')}
            </h2>
          </div>
          <button
            aria-label="关闭弹窗"
            disabled={submitting}
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-secondary text-muted-foreground hover:text-foreground transition-colors disabled:cursor-wait disabled:opacity-40"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 模式 toggle(SegmentedControl 风格) */}
        <div className="mb-4 inline-flex rounded-lg bg-muted p-0.5 border border-border/70 self-start">
          <button
            type="button"
            disabled={submitting}
            onClick={() => changeMode('single')}
            className={`
              inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-md text-xs font-medium transition-all
              ${mode === 'single'
                ? 'bg-gradient-to-br from-gold to-primary text-primary-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
              }
            `}
          >
            <User className="w-3.5 h-3.5" />
            {t('mode.single')}
          </button>
          <button
            type="button"
            disabled={submitting}
            onClick={() => changeMode('group')}
            className={`
              inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-md text-xs font-medium transition-all
              ${mode === 'group'
                ? 'bg-gradient-to-br from-gold to-primary text-primary-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
              }
            `}
          >
            <Users className="w-3.5 h-3.5" />
            {t('mode.group')}
          </button>
        </div>

        <p className="text-[10px] text-muted-foreground mb-3 -mt-2">
          {mode === 'single' ? t('mode.singleHint') : t('mode.groupHint')}
        </p>

        {/* 提示:无道人 */}
        {agents.length === 0 && (
          <div className="text-center py-10 text-xs text-muted-foreground">
            {t('mode.noAgents')}
          </div>
        )}

        {agentError && (
          <div className="mb-3">
            <LoadNotice
              title={t('load.agentsError')}
              message={agentError}
              retryLabel={t('load.retry')}
              onRetry={onRetryAgents}
            />
          </div>
        )}

        {/* 列表 */}
        <div className="space-y-2 flex-1 overflow-y-auto min-h-0">
          {agents.map(agent => {
            const checked = selected.has(agent.id)
            return (
              <button
                key={agent.id}
                type="button"
                aria-pressed={checked}
                disabled={submitting}
                onClick={() => {
                  if (mode === 'single') {
                    onSelectionChange()
                    setSelected(new Set([agent.id]))
                    void onSelectSingle(agent.id)
                  } else {
                    toggleSelect(agent.id)
                  }
                }}
                className={`
                  w-full flex items-center gap-3 p-3 rounded-lg text-left transition-all
                  ${checked
                    ? 'bg-gold/15 border border-gold/50'
                    : 'bg-muted border border-border/70 hover:border-gold/40 hover:bg-gold/5'
                  }
                  disabled:cursor-wait disabled:opacity-60
                `}
              >
                {/* 群模式显示多选 checkbox,单模式隐藏 */}
                {mode === 'group' && (
                  <div className={`
                    w-5 h-5 rounded border-2 flex items-center justify-center shrink-0 transition-colors
                    ${checked ? 'bg-gold border-gold' : 'border-border bg-card'}
                  `}>
                    {checked && <span className="text-primary-foreground text-xs font-bold">✓</span>}
                  </div>
                )}
                <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white font-serif font-bold flex-shrink-0">
                  {agent.name.charAt(0)}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground">{agent.name}</p>
                  <p className="text-xs text-muted-foreground truncate">{agent.model_name}</p>
                </div>
                {mode === 'single' && <Sparkles className="w-4 h-4 text-gold/50" />}
              </button>
            )
          })}
        </div>

        {/* 群模式:底部确认按钮 */}
        {mode === 'group' && (
          <div className="mt-4 pt-4 border-t border-border/70">
            <button
              type="button"
              onClick={handleConfirm}
              disabled={selected.size < 2 || submitting}
              className="dao-btn-primary w-full disabled:cursor-not-allowed disabled:opacity-40"
            >
              {t('mode.confirm')} ({selected.size})
            </button>
            {selected.size < 2 && (
              <p className="text-[10px] text-muted-foreground text-center mt-1.5">
                {t('mode.selectAtLeast', { n: 2 })}
              </p>
            )}
          </div>
        )}

        {launchState.status !== 'idle' && (
          <div className="mt-3 rounded-lg border border-gold/40 bg-gold/5 px-3 py-2.5 shadow-sm">
            {launchState.status === 'submitting' ? (
              <ActionFeedback status="submitting" message={t('launch.submitting')} />
            ) : (
              <>
                <ActionFeedback
                  status="error"
                  message={launchState.message}
                  onRetry={() => { void retry() }}
                  retryLabel={t('launch.retry')}
                />
                {launchState.errorCode === 'service.chat.model_unavailable' && (
                  <Link
                    href="/settings"
                    className="mt-2 inline-flex max-w-full break-words text-left text-xs font-medium whitespace-normal text-gold hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                  >
                    {t('launch.modelSettings')}
                  </Link>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/* ========== 子组件:输入框(@ 补全集成) ========== */

function ChatInput({
  value,
  onChange,
  onSend,
  streaming,
  onStop,
  isGroup,
  members,
  disabled,
  disabledReason,
}: {
  value: string
  onChange: (v: string) => void
  onSend: () => void
  streaming: boolean
  onStop: () => void
  isGroup: boolean
  members: import('@/services/types').GroupMember[]
  disabled: boolean
  disabledReason: string | null
}) {
  const t = useTranslations('chatView')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // @ 提及补全:从光标前找 @ 查询
  // 仅群聊启用(单聊时 members 为空)
  const [mention, setMention] = useState<{
    query: string
    start: number // @ 字符在 textarea.value 中的位置
    activeIndex: number
  } | null>(null)
  const [popPos, setPopPos] = useState<{ bottom: number; left: number } | null>(null)

  // 候选项:群聊时仅列群成员,单聊时不用(@补全不启用)
  const candidates = useMemo(() => {
    if (!mention) return []
    const q = mention.query.toLowerCase()
    if (isGroup) {
      return members.filter(m => m.name.toLowerCase().includes(q))
    }
    return []
  }, [mention, isGroup, members])

  const safeActiveIndex = Math.min(mention?.activeIndex || 0, Math.max(0, candidates.length - 1))

  // 监听 onChange,检测 @ 触发
  const updateMention = (next: string, caret: number) => {
    if (!isGroup) {
      setMention(null)
      setPopPos(null)
      return
    }
    // 从 caret 往前找最近 @
    const before = next.slice(0, caret)
    const match = before.match(/(^|\s)@([^\s@]*)$/)
    if (!match) {
      setMention(null)
      setPopPos(null)
      return
    }
    const start = match.index! + match[1].length // '@' 位置
    setMention({ query: match[2], start, activeIndex: 0 })
    const rect = textareaRef.current?.getBoundingClientRect()
    if (rect) {
      setPopPos({ bottom: window.innerHeight - rect.top + 4, left: rect.left })
    }
  }

  // 替换 @query 为 @name + 空格,光标移到 name 之后
  const applyMention = (name: string) => {
    if (!mention) return
    const before = value.slice(0, mention.start)
    const after = value.slice(textareaRef.current?.selectionStart || mention.start + 1 + mention.query.length)
    const inserted = `@${name} `
    const next = before + inserted + after
    onChange(next)
    setMention(null)
    setPopPos(null)
    // 重新聚焦 + 设置光标
    requestAnimationFrame(() => {
      if (textareaRef.current) {
        const pos = (before + inserted).length
        textareaRef.current.focus()
        textareaRef.current.setSelectionRange(pos, pos)
      }
    })
  }

  // 键盘:上下选择 / Enter 选中 / Esc 关闭
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // 补全浮层开启时拦截
    if (mention && candidates.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setMention(m => m ? { ...m, activeIndex: (m.activeIndex + 1) % candidates.length } : null)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setMention(m => m ? { ...m, activeIndex: (m.activeIndex - 1 + candidates.length) % candidates.length } : null)
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        const pick = candidates[safeActiveIndex]
        if (pick) applyMention(pick.name)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setMention(null)
        return
      }
    }
    // Enter 发送(非 IME)
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      onSend()
    }
  }

  return (
    <div className="px-4 py-3 border-t border-border/70 bg-card/80">
      <div className="flex items-end gap-2">
        <div ref={containerRef} className="relative flex-1 min-w-0">
          <textarea
            ref={textareaRef}
            value={value}
            onChange={e => {
              onChange(e.target.value)
              updateMention(e.target.value, e.target.selectionStart || 0)
            }}
            onKeyDown={handleKeyDown}
            disabled={disabled}
            onBlur={() => {
              // 延迟关闭,允许 click 浮层 item
              setTimeout(() => setMention(null), 150)
            }}
            aria-label={t('input.messageLabel')}
            placeholder={disabled ? t('input.readOnlyPlaceholder') : t(isGroup ? 'input.groupPlaceholder' : 'input.placeholder')}
            className="dao-input resize-none min-h-[44px] max-h-[120px] py-2.5 w-full"
            rows={1}
          />
          {/* @ 补全浮层(飞书式) */}
          {mention && popPos && (
            <MentionSuggest
              candidates={candidates}
              activeIndex={safeActiveIndex}
              onPick={(name) => applyMention(name)}
              onHover={(i) => setMention(m => m ? { ...m, activeIndex: i } : null)}
              position={popPos}
            />
          )}
        </div>
        <button
          aria-label={streaming ? t('input.stop') : t('input.send')}
          onClick={streaming ? onStop : onSend}
          disabled={!streaming && (disabled || !value.trim())}
          className="dao-btn-primary px-3 py-2.5 flex-shrink-0 disabled:opacity-40"
          title={streaming ? t('input.stop') : t('input.send')}
        >
          {streaming ? (
            <Square className="w-5 h-5" />
          ) : (
            <Send className="w-5 h-5" />
          )}
        </button>
      </div>
      <p className={`mt-1.5 break-words text-center text-[10px] ${disabledReason ? 'text-gold/80' : 'text-sage/70'}`}>
        {disabledReason || t(isGroup ? 'input.helpGroup' : 'input.help')}
      </p>
    </div>
  )
}

/* ========== 子组件:群成员面板(全局事件触发) ========== */

function GroupMembersPanelSession() {
  const { state: chatState } = useChat()
  const [open, setOpen] = useState(false)

  useEffect(() => {
    const handler = () => setOpen(true)
    window.addEventListener('open-group-members', handler)
    return () => window.removeEventListener('open-group-members', handler)
  }, [])

  if (!chatState.currentSession || chatState.currentSession.type !== 'group') return null
  return (
    <GroupMembersPanel
      session={chatState.currentSession}
      open={open}
      onClose={() => setOpen(false)}
    />
  )
}

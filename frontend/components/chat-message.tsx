'use client'

/**
 * 聊天消息气泡组件：稳定单列的“丹房议事桌”。身份、正文和流式状态都在
 * 同一消息块内，群聊切换发言人时不会改变列宽或挤压相邻内容。
 *
 * 流式性能:
 *   - streaming=true:  走纯文本路径(见 MarkdownRenderer),无 markdown 解析,逐字显示
 *   - streaming=false: 走完整 markdown 渲染(代码高亮、表格、列表等)
 *
 * 交互增强(chat UX polish):
 *   - 用户/道人头像 click → 触发 ProfilePopover(飞书式浮窗,显示简介)
 *   - 消息里 @xxx 文本高亮且可点击,点击再次打开对应道人的 popover
 *   - 群聊 mentions chips 也可点击,同上
 */
import { useRef, useState, useMemo } from 'react'
import { useTranslations } from 'next-intl'
import { TriangleAlert, CircleStop, Code2, ChevronDown } from 'lucide-react'
import type { ChatMessage as ChatMessageType, Agent } from '@/services/types'
import { MarkdownRenderer } from '@/components/markdown-renderer'
import { ProfilePopover } from '@/components/profile-popover'
import { EntityAvatar } from '@/components/avatar/entity-avatar'
import { useUser } from '@/contexts/UserContext'
import { useAgent } from '@/contexts/AgentContext'
import { useChat } from '@/contexts/ChatContext'

interface ChatMessageProps {
  message: ChatMessageType
  /** 是否正在流式输出中 */
  streaming?: boolean
  /** 群聊: 成员列表(用于 @ 提及查名字) */
  members?: import('@/services/types').GroupMember[]
  /** 断线/上游失败后的重新发送动作。只读历史不传入。 */
  onRetry?: () => void
}

export function ChatMessage({ message, streaming = false, members, onRetry }: ChatMessageProps) {
  const t = useTranslations('chatMessage')
  const tGroup = useTranslations('groupChat')

  // 头像 popover 状态
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [promptDebugOpen, setPromptDebugOpen] = useState(false)
  const avatarAnchorRef = useRef<HTMLButtonElement>(null)

  const isUser = message.role === 'user'

  // 头像 popover 数据
  const { profile: userProfile } = useUser()
  const { state: agentState } = useAgent()
  const { state: chatState, continueRun } = useChat()
  const currentSession = chatState.currentSession
  // 单聊会话身份兜底(群聊不得使用单聊的 session 身份;群聊 agent_name/agent_avatar 为空)
  const sessionAgentName = currentSession && currentSession.type !== 'group' ? currentSession.agent_name : undefined
  const sessionAgentAvatar = currentSession && currentSession.type !== 'group' ? currentSession.agent_avatar : undefined

  /**
   * 查找消息对应道人的完整档案(用于 popover 展示 personality / model_name / proactivity)
   * - 群聊:message.agent_id 是该条消息的发言道人
   * - 单聊:消息无 agent_id,需从 currentSession.agent_id 查
   * 找不到时降级:用 message.agent_name + agents 中按 name 模糊匹配
   */
  const agentProfile: Agent | null = useMemo(() => {
    if (isUser) return null
    const all = agentState.agents
    // 1) 群聊:用 message.agent_id
    if (message.agent_id) {
      const found = all.find(a => a.id === message.agent_id)
      if (found) return found
    }
    // 2) 单聊 / 群聊回退:用 message.agent_name 匹配
    if (message.agent_name) {
      const byName = all.find(a => a.name === message.agent_name)
      if (byName) return byName
    }
    // 3) 单聊:用 currentSession.agent_id
    const sessionAgentId = chatState.currentSession?.agent_id
    if (sessionAgentId) {
      const fromSession = all.find(a => a.id === sessionAgentId)
      if (fromSession) return fromSession
    }
    // 4) 当前会话新字段足以生成简略档案，确保头像始终可点。
    const session = chatState.currentSession
    if (session && session.type !== 'group' && session.agent_name && (!message.agent_id || session.agent_id === message.agent_id)) {
      return {
        id: session.agent_id,
        name: session.agent_name,
        avatar: session.agent_avatar,
        personality: '',
        model_name: '',
        status: session.agent_status ?? 'active',
        proactivity: 0,
        created_at: session.updated_at || session.created_at,
      }
    }
    const member = members?.find(x => x.agent_id === message.agent_id)
    if (member) {
      return {
        id: member.agent_id,
        name: member.name,
        avatar: member.avatar,
        personality: '',
        model_name: '',
        status: 'active',
        proactivity: member.proactivity,
        created_at: message.created_at,
      }
    }
    return null
  }, [isUser, message.agent_id, message.agent_name, message.created_at, agentState.agents, chatState.currentSession, members])

  // Task 14:该消息是当前 interrupted run 的不完整片段时,提供「继续」续跑动作
  // (重发/重试路径另行保留;续跑按 run_id 定位,不重发用户消息)
  const canContinue = !isUser
    && Boolean(message.incomplete)
    && Boolean(message.run_id)
    && chatState.interruptedRunId === message.run_id

  const memberAvatar = members?.find(member => member.agent_id === message.agent_id)?.avatar
  const avatarSrc = isUser ? userProfile?.avatar : (message.agent_avatar || agentProfile?.avatar || memberAvatar || sessionAgentAvatar)

  // Hook 必须在所有渲染分支中保持同序；通知条在档案数据解析后再提前返回。
  if (message.role === 'system' && !message.is_error) {
    return (
      <div className="flex justify-center animate-in fade-in duration-300 my-1">
        <span className="text-[10px] text-muted-foreground bg-muted px-3 py-1 rounded-full">
          {message.content}
        </span>
      </div>
    )
  }

  return (
    <div className={`
      flex w-full items-start gap-3 min-w-0
      ${isUser ? 'flex-row-reverse' : 'flex-row'}
      animate-in fade-in duration-300
    `}>
      {/* 头像(金刚位):固定顶端,不被气泡高度拉长 */}
      <button
        ref={avatarAnchorRef}
        type="button"
        aria-label={isUser ? t('userLabel') : (message.agent_name || sessionAgentName || t('assistantLabel'))}
        onClick={() => setPopoverOpen(true)}
        className={`
          shrink-0 self-start
          w-10 h-10 rounded-xl
          flex items-center justify-center
          transition-all duration-150
          hover:ring-2 hover:ring-gold/50 hover:ring-offset-2 hover:ring-offset-background
          focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60
          ${isUser
            ? 'bg-primary/10 text-primary border border-primary/30'
            : 'bg-gold/15 text-gold border border-gold/30'
          }
        `}
      >
        {/* 头像(金刚位):固定顶端,不被气泡高度拉长;失败/空值统一回退首字或 Bot 图标 */}
        <EntityAvatar
          name={isUser
            ? (userProfile?.display_name || t('userLabel'))
            : (message.agent_name || sessionAgentName || t('assistantLabel'))}
          src={avatarSrc}
          size="sm"
          fallback={isUser ? 'initial' : (message.agent_name || sessionAgentName) ? 'initial' : 'bot'}
        />
      </button>

      {/* 名字 + 气泡(竖直堆叠,与头像独立列) */}
      <div className={`
        flex-1 min-w-0 flex flex-col
        ${isUser ? 'items-end' : 'items-start'}
      `}>
        {/* 角色标签:单行,block,与气泡独立行 */}
        <span className={`
          block text-[11px] mb-1.5 px-1 whitespace-nowrap font-medium
          ${isUser ? 'self-end' : 'self-start'}
          ${isUser
            ? 'text-primary/75'
            : 'text-gold/90'
          }
        `}>
          {isUser
            ? (userProfile?.display_name || t('userLabel'))
            : (message.agent_name || sessionAgentName || t('assistantLabel'))}
        </span>

        {/* 消息气泡:block,宽由 max-w 控制,长内容自然换行 */}
        <div className={`
          relative block w-fit max-w-[82%] text-left
          px-4 py-3 rounded-2xl break-words shadow-sm
          ${isUser
            ? 'bg-primary/[0.07] border border-primary/25 rounded-tr-md'
            : 'bg-card border border-border/80 rounded-tl-md'
          }
        `}>
          {/* 丹色印记：一条克制的身份线，不参与正文宽度计算。 */}
          {!isUser && (
            <>
              <div className="absolute left-0 top-3 bottom-3 w-0.5 bg-gradient-to-b from-gold/80 to-sage/50 rounded-full" />
            </>
          )}

          {/* 消息内容 */}
          <div className={`md-selectable ${isUser ? '' : 'pl-2 pr-2'} min-w-0 break-words`}>
            {isUser ? (
              // 用户消息: 高亮 @名字 且 @ 文字可点击触发 popover
              <p className="text-sm text-foreground whitespace-pre-wrap leading-relaxed">
                {message.content.split(/(@[^\s@，。,.!?？！:：;；]+)/g).map((part, i) =>
                  /^@[^\s@，。,.!?？！:：;；]+$/.test(part)
                    ? <MentionChip key={i} name={part.slice(1)} members={members} variant="agent" />
                    : <span key={i}>{part}</span>
                )}
              </p>
            ) : (
              // 流中走纯文本路径(streaming=true);流结束后走完整 markdown
              <MarkdownRenderer content={message.content} streaming={streaming} />
            )}
          </div>

          {/* 流式输出光标 — 仅流中、仅 AI 消息 */}
          {streaming && !isUser && (
            <span className="inline-block w-1.5 h-4 rounded-full bg-gold/80 ml-1 align-text-bottom animate-pulse" />
          )}
        </div>

        {!isUser && message.prompt_debug && (
          <div className="mt-1.5 w-full max-w-[82%] pl-1">
            <button
              type="button"
              aria-expanded={promptDebugOpen}
              onClick={() => setPromptDebugOpen(open => !open)}
              className="inline-flex items-center gap-1.5 text-[10px] text-muted-foreground transition-colors hover:text-gold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
            >
              <Code2 className="h-3 w-3" />
              {promptDebugOpen ? t('promptDebugHide') : t('promptDebugShow')}
              <ChevronDown className={`h-3 w-3 transition-transform ${promptDebugOpen ? 'rotate-180' : ''}`} />
            </button>
            {promptDebugOpen && (
              <div className="mt-2 max-h-96 overflow-auto rounded-xl border border-border/70 bg-muted/50 p-3 text-left text-[11px] leading-relaxed">
                <div className="mb-3 flex flex-wrap gap-x-4 gap-y-1 text-muted-foreground">
                  <span>{t('promptDebugModel')}: <code className="text-foreground">{message.prompt_debug.model}</code></span>
                  {(() => {
                    // LangGraph 无预算概念时后端发 0/0,此时预算行无意义不展示;
                    // generation 缺失(异常/旧载荷)也不得崩渲染(2026-09-04 展开面板 TypeError 根因)。
                    const gen = message.prompt_debug.generation
                    if (!gen || (gen.max_tokens <= 0 && gen.max_sentences <= 0)) return null
                    return (
                      <span>{t('promptDebugBudget')}: <code className="text-foreground">{gen.max_tokens} tokens / {gen.max_sentences} sentences</code></span>
                    )
                  })()}
                </div>
                <div className="space-y-3">
                  {message.prompt_debug.messages.map((item, index) => (
                    <div key={`${item.role}-${index}`}>
                      <div className="mb-1 font-mono uppercase tracking-wide text-gold">{item.role}</div>
                      <pre className="whitespace-pre-wrap break-words font-mono text-foreground">{item.content}</pre>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* @提及 chips(群聊道人消息) - 可点击 */}
        {!isUser && message.mentions && (message.mentions.agents?.length || message.mentions.user) && (
          <div className="flex items-center gap-1.5 mt-1.5 pl-1 flex-wrap">
            <span className="text-[10px] text-muted-foreground">{tGroup('mentioned')}</span>
            {message.mentions.user && (
              <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-primary/10 text-primary/80">
                @{tGroup('userLabel')}
              </span>
            )}
            {(message.mentions.agents || []).map(uuid => {
              const m = members?.find(x => x.agent_id === uuid)
              if (!m) return null
              return (
                <span
                  key={uuid}
                  className="text-[10px] px-1.5 py-0.5 rounded-full bg-gold/10 text-gold/80"
                >
                  @{m.name}
                </span>
              )
            })}
          </div>
        )}

        {/* 状态标记(仅 AI 消息) */}
        {!isUser && (message.incomplete || message.stopped) && (
          <div className="flex items-center gap-3 mt-1.5 pl-1 flex-wrap">
            {message.incomplete && (
              <>
                <span className="flex items-center gap-1 text-[10px] text-gold/80 whitespace-nowrap">
                  <TriangleAlert className="w-3 h-3 shrink-0" />
                  {t('incomplete')}
                </span>
                {canContinue && (
                  <button
                    type="button"
                    onClick={() => { void continueRun() }}
                    className="text-[10px] font-medium text-gold hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                  >
                    {t('continue')}
                  </button>
                )}
                {onRetry && (
                  <button
                    type="button"
                    onClick={onRetry}
                    className="text-[10px] font-medium text-gold hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                  >
                    {t('retry')}
                  </button>
                )}
              </>
            )}
            {message.stopped && (
              <span className="flex items-center gap-1 text-[10px] text-muted-foreground whitespace-nowrap">
                <CircleStop className="w-3 h-3 shrink-0" />
                {t('stopped')}
              </span>
            )}
          </div>
        )}
      </div>

      {/* 头像 popover */}
      <ProfilePopover
        kind={isUser ? 'user' : 'agent'}
        anchorRef={avatarAnchorRef}
        open={popoverOpen}
        onClose={() => setPopoverOpen(false)}
        userProfile={userProfile}
        agent={agentProfile}
      />
    </div>
  )
}

/**
 * @ 提及高亮 chip - 单聊模式下可点击,点击后弹该道人的 popover
 * 通过 members(群聊)/agents(单聊)查找对应 agent 并打开 popover
 */
function MentionChip({
  name,
  members,
  variant,
}: {
  name: string
  members?: import('@/services/types').GroupMember[]
  variant: 'user' | 'agent'
}) {
  const { state: agentState } = useAgent()
  const { state: chatState } = useChat()
  /**
   * @ 提及目标查找 - 兜底链:
   *  1) 群聊 members 按 name 找 → agent_id
   *  2) agents 按 name 精确匹配
   *  3) agents 按 name includes 模糊匹配(用户可能输入简写)
   *  4) 单聊 currentSession.agent_id 兜底(只有一个道人,任意 @ 都指向它)
   *  variant='user' 永远返回 null(用户没有 popover)
   */
  const target = useMemo(() => {
    if (variant === 'user') return null
    const all = agentState.agents
    // 1) 群聊 members
    if (members) {
      const m = members.find(x => x.name === name)
      if (m) {
        return all.find(a => a.id === m.agent_id) || null
      }
    }
    // 2) agents 精确
    const exact = all.find(a => a.name === name)
    if (exact) return exact
    // 3) agents 模糊(单聊场景下用户输入简写)
    if (all.length > 0) {
      const fuzzy = all.find(a => a.name.includes(name) || name.includes(a.name))
      if (fuzzy) return fuzzy
    }
    // 4) 单聊兜底:任何 @ 都指向 currentSession 那个道人
    const sessionAgentId = chatState.currentSession?.agent_id
    if (sessionAgentId) {
      return all.find(a => a.id === sessionAgentId) || null
    }
    return null
  }, [name, members, variant, agentState.agents, chatState.currentSession?.agent_id])

  const popoverAnchorRef = useRef<HTMLButtonElement>(null)
  const [popoverOpen, setPopoverOpen] = useState(false)

  return (
    <>
      <button
        ref={popoverAnchorRef}
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          if (target) setPopoverOpen(true)
        }}
        className="text-gold font-medium hover:underline cursor-pointer"
        title={target ? `查看 ${name} 的简介` : name}
      >
        @{name}
      </button>
      {target && (
        <ProfilePopover
          kind="agent"
          anchorRef={popoverAnchorRef}
          open={popoverOpen}
          onClose={() => setPopoverOpen(false)}
          agent={target}
        />
      )}
    </>
  )
}

'use client'

/**
 * 聊天消息气泡组件 - 浅色宣纸卷轴风
 * 用户消息: 右侧，朱砂红边框
 * AI 消息: 左侧，金色边框，卷轴风格
 *
 * 版式（飞书/微信 金刚位）:
 *   ┌─────────────────────────────────────────────┐
 *   │ [头像]  [名字]                              │  ← 头像固定顶端
 *   │          ┌──────────────────────────────┐  │
 *   │          │ 气泡(可换行,可很长)            │  │
 *   │          │                              │  │
 *   │          └──────────────────────────────┘  │
 *   │          [状态/mentions chips]              │
 *   └─────────────────────────────────────────────┘
 *
 *   - items-start: 头像始终对齐到本行顶端(不被气泡高度拉长)
 *   - 名字 block: 与气泡竖直堆叠,不挤在一行
 *   - 气泡 block: 宽由 max-w 限制,长内容自然换行
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
import { User, Bot, TriangleAlert, CircleStop } from 'lucide-react'
import type { ChatMessage as ChatMessageType, Agent } from '@/services/types'
import { MarkdownRenderer } from '@/components/markdown-renderer'
import { ProfilePopover } from '@/components/profile-popover'
import { useUser } from '@/contexts/UserContext'
import { useAgent } from '@/contexts/AgentContext'
import { useChat } from '@/contexts/ChatContext'

interface ChatMessageProps {
  message: ChatMessageType
  /** 是否正在流式输出中 */
  streaming?: boolean
  /** 群聊: 成员列表(用于 @ 提及查名字) */
  members?: import('@/services/types').GroupMember[]
}

export function ChatMessage({ message, streaming = false, members }: ChatMessageProps) {
  const t = useTranslations('chatMessage')
  const tGroup = useTranslations('groupChat')

  // 头像 popover 状态
  const [popoverOpen, setPopoverOpen] = useState(false)
  const avatarAnchorRef = useRef<HTMLButtonElement>(null)

  // 群聊 system 通知条(成员变动 / 整轮沉默)
  if (message.role === 'system' && !message.is_error) {
    return (
      <div className="flex justify-center animate-in fade-in duration-300 my-1">
        <span className="text-[10px] text-muted-foreground bg-muted px-3 py-1 rounded-full">
          {message.content}
        </span>
      </div>
    )
  }

  const isUser = message.role === 'user'

  // 头像 popover 数据
  const { profile: userProfile } = useUser()
  const { state: agentState } = useAgent()
  const { state: chatState } = useChat()

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
    // 4) 群聊 members 中也存了 agent_id/name,做最后兜底
    if (members && message.agent_id) {
      const m = members.find(x => x.agent_id === message.agent_id)
      if (m) {
        return all.find(a => a.id === m.agent_id) || null
      }
    }
    return null
  }, [isUser, message.agent_id, message.agent_name, agentState.agents, chatState.currentSession?.agent_id, members])

  return (
    <div className={`
      flex items-start gap-3 md:gap-4 min-w-0
      ${isUser ? 'flex-row-reverse' : 'flex-row'}
      animate-in fade-in duration-300
    `}>
      {/* 头像(金刚位):固定顶端,不被气泡高度拉长 */}
      <button
        ref={avatarAnchorRef}
        type="button"
        aria-label={isUser ? t('userLabel') : (message.agent_name || t('assistantLabel'))}
        onClick={() => setPopoverOpen(true)}
        className={`
          shrink-0 self-start
          w-9 h-9 md:w-10 md:h-10 rounded-full
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
        {isUser
          ? (userProfile?.avatar
              ? // eslint-disable-next-line @next/next/no-img-element
                <img src={userProfile.avatar} alt={userProfile.display_name} className="w-full h-full rounded-full object-cover" />
              : <User className="w-5 h-5" />)
          : (message.agent_name
              ? <span className="font-serif font-bold">{message.agent_name.charAt(0)}</span>
              : <Bot className="w-5 h-5" />)}
      </button>

      {/* 名字 + 气泡(竖直堆叠,与头像独立列) */}
      <div className={`
        flex-1 min-w-0 flex flex-col
        ${isUser ? 'items-end' : 'items-start'}
      `}>
        {/* 角色标签:单行,block,与气泡独立行 */}
        <span className={`
          block text-[10px] mb-1.5 px-2 py-0.5 rounded-full whitespace-nowrap
          ${isUser ? 'self-end' : 'self-start'}
          ${isUser
            ? 'bg-primary/10 text-primary/70'
            : 'bg-gold/10 text-gold/80'
          }
        `}>
          {isUser
            ? (userProfile?.display_name || t('userLabel'))
            : (message.agent_name || t('assistantLabel'))}
        </span>

        {/* 消息气泡:block,宽由 max-w 控制,长内容自然换行 */}
        <div className={`
          relative block text-left
          max-w-[88%] sm:max-w-[78%] md:max-w-[68%] lg:max-w-[60%]
          px-4 py-3 rounded-2xl break-words
          ${isUser
            ? 'bg-primary/5 border border-primary/30 rounded-tr-sm'
            : 'bg-card/90 border border-gold/30 rounded-tl-sm'
          }
        `}>
          {/* 卷轴装饰(仅 AI 消息) */}
          {!isUser && (
            <>
              <div className="absolute -left-1 top-2 bottom-2 w-1 bg-gradient-to-b from-gold/60 via-gold/40 to-gold/60 rounded-full" />
              <div className="absolute -right-1 top-2 bottom-2 w-1 bg-gradient-to-b from-gold/60 via-gold/40 to-gold/60 rounded-full" />
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
            <span className="inline-block w-2 h-4 bg-gold ml-1 align-text-bottom animate-pulse" />
          )}
        </div>

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
              <span className="flex items-center gap-1 text-[10px] text-gold/80 whitespace-nowrap">
                <TriangleAlert className="w-3 h-3 shrink-0" />
                {t('incomplete')}
              </span>
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

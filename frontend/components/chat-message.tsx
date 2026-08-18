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
    // 4) 会话或群成员数据足以生成简略档案，确保头像始终可点。
    const sessionAgent = chatState.currentSession?.agent
    if (sessionAgent && (!message.agent_id || sessionAgent.id === message.agent_id)) return sessionAgent
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

  const memberAvatar = members?.find(member => member.agent_id === message.agent_id)?.avatar
  const avatarSrc = isUser ? userProfile?.avatar : (agentProfile?.avatar || memberAvatar)

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
        aria-label={isUser ? t('userLabel') : (message.agent_name || t('assistantLabel'))}
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
        {avatarSrc
          ? // eslint-disable-next-line @next/next/no-img-element
            <img src={avatarSrc} alt={isUser ? userProfile?.display_name : agentProfile?.name} className="h-full w-full rounded-xl object-cover" />
          : isUser
            ? <User className="w-5 h-5" />
            : message.agent_name
              ? <span className="font-serif font-bold">{message.agent_name.charAt(0)}</span>
              : <Bot className="w-5 h-5" />}
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
            : (message.agent_name || t('assistantLabel'))}
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
          <div className={`md-selectable ${isUser ? '' : 'pl-1.5'} min-w-0 break-words`}>
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

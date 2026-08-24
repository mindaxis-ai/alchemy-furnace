'use client'

/**
 * 道人卡片组件 - 浅色宣纸风
 * 默认模式整卡为单一键盘可访问的导航容器(role="link"),点击/Enter/Space 进入详情;
 * 内部「论道」按钮阻止冒泡,避免双重导航:
 * - active: 可发起会话(useChatLaunchFlow)
 * - inactive: 显示已停用徽记、论道按钮禁用并带原因提示,但整卡仍可进入详情(详情页可恢复)
 * 紧凑模式保持整卡 Link(无内部按钮)
 */
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import { ChevronRight, Cpu, Loader2, MessageSquare, Sparkles } from 'lucide-react'
import { useChatLaunchFlow } from '@/hooks/use-chat-launch-flow'
import type { Agent } from '@/services/types'
import { truncateText } from '@/utils/format'

interface AgentCardProps {
  agent: Agent
  /** 紧凑模式 */
  compact?: boolean
}

/** 生成头像渐变颜色（根据名称确定性生成） */
function getAvatarColor(name: string): string {
  const colors = [
    'from-primary to-primary/70',
    'from-sage to-sage/70',
    'from-gold to-gold/70',
    'from-blue-500 to-blue-700',
    'from-purple-500 to-purple-700',
    'from-teal-500 to-teal-700',
  ]
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return colors[Math.abs(hash) % colors.length]
}

export function AgentCard({ agent, compact = false }: AgentCardProps) {
  const t = useTranslations('agentCard')
  const tStatus = useTranslations('agentCard.status')
  const router = useRouter()
  const launchFlow = useChatLaunchFlow()

  const inactive = agent.status !== 'active'

  // 状态映射
  const statusInfo: Record<string, { className: string }> = {
    active: { className: 'bg-sage/20 text-sage border-sage/30' },
    inactive: { className: 'bg-muted text-muted-foreground border-border/70' },
  }
  const statusClass = (statusInfo[agent.status] || statusInfo.inactive).className
  const statusLabel = agent.status === 'active' ? tStatus('active') : tStatus('inactive')

  const avatarGradient = getAvatarColor(agent.name)
  const href = `/agents/${agent.id}`

  if (compact) {
    // 紧凑模式:整卡 Link,无内部按钮
    return (
      <Link
        href={href}
        className="dao-card group flex min-w-0 items-center gap-3 p-4 sm:gap-4"
      >
        {/* 头像 */}
        <div className={`
          flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br ${avatarGradient}
          font-serif text-lg font-bold text-primary-foreground
          shadow-lg
        `}>
          {agent.name.charAt(0)}
        </div>

        {/* 信息 */}
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <h3 className="min-w-0 truncate font-serif font-semibold text-foreground transition-colors group-hover:text-gold">
              {agent.name}
            </h3>
            <span className={`shrink-0 whitespace-nowrap rounded-full border px-2 py-0.5 text-[10px] ${statusClass}`}>
              {statusLabel}
            </span>
          </div>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {agent.personality ? truncateText(agent.personality, 40) : t('noPersona')}
          </p>
        </div>

        {/* 模型 */}
        <div className="hidden shrink-0 items-center gap-3 text-xs text-muted-foreground sm:flex">
          <span className="flex items-center gap-1 whitespace-nowrap">
            <Cpu className="h-3.5 w-3.5" />
            <span className="max-w-[12ch] truncate">{agent.model_name}</span>
          </span>
        </div>

        <ChevronRight className="h-5 w-5 shrink-0 text-sage transition-colors group-hover:text-gold" />
      </Link>
    )
  }

  // 默认模式:整卡键盘可达导航 + 内部论道按钮
  const navigate = () => router.push(href)

  return (
    <div
      role="link"
      tabIndex={0}
      aria-label={agent.name}
      onClick={navigate}
      onKeyDown={(event) => {
        // 仅当按键源自卡片本身才导航;焦点在内部按钮上时放行,
        // 交由按钮自身的键盘激活(其 onClick 已 stopPropagation)
        if (event.target !== event.currentTarget) return
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          navigate()
        }
      }}
      className="dao-card group relative flex h-full min-w-0 cursor-pointer flex-col p-5 transition-shadow focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
    >
      {/* 顶部：头像 + 状态 */}
      <div className="mb-4 flex items-start justify-between gap-2">
        <div className={`
          flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br ${avatarGradient}
          font-serif text-2xl font-bold text-primary-foreground
          shadow-lg transition-transform duration-300 group-hover:scale-105
        `}>
          {agent.name.charAt(0)}
        </div>
        <div className="flex min-w-0 items-center gap-2">
          {agent.status === 'active' && (
            <Sparkles className="h-4 w-4 shrink-0 animate-pulse text-gold" />
          )}
          <span className={`shrink-0 whitespace-nowrap rounded-full border px-2 py-0.5 text-[10px] ${statusClass}`}>
            {statusLabel}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <h3 className="mb-1.5 truncate font-serif text-lg font-bold text-foreground transition-colors group-hover:text-gold">
        {agent.name}
      </h3>

      {/* 性格描述 */}
      <p className="mb-4 line-clamp-3 flex-1 text-sm text-muted-foreground">
        {agent.personality || t('noPersona')}
      </p>

      {/* 底部分隔 */}
      <div className="dao-divider my-3 text-[10px]">
        <Sparkles className="h-3 w-3" />
      </div>

      {/* 底部信息:模型 + 论道入口 */}
      <div className="flex min-w-0 items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="flex min-w-0 items-center gap-1 truncate">
          <Cpu className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{agent.model_name}</span>
        </span>
        <button
          type="button"
          disabled={inactive || launchFlow.state.status === 'submitting'}
          title={inactive ? t('inactiveChatHint') : t('chatTitle', { name: agent.name })}
          onClick={(event) => {
            // 阻止冒泡到整卡导航容器,避免双重导航
            event.stopPropagation()
            if (inactive) return
            void launchFlow.launchSingle(agent.id)
          }}
          className="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs text-gold/80 transition-colors hover:text-gold disabled:cursor-not-allowed disabled:text-muted-foreground/50 disabled:hover:text-muted-foreground/50"
        >
          {launchFlow.state.status === 'submitting' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <MessageSquare className="h-3.5 w-3.5" />
          )}
          {t('chatCta')}
        </button>
      </div>

      {/* 发起会话失败:不静默,卡片上给出错误 */}
      {launchFlow.state.status === 'error' && (
        <p role="alert" className="mt-2 break-words text-[11px] text-primary">
          {launchFlow.state.message}
        </p>
      )}
    </div>
  )
}

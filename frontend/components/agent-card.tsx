'use client'

/**
 * 道人卡片组件 - 浅色宣纸风
 * 显示头像占位、名称、性格摘要、模型、状态标识
 * 响应式：桌面端网格卡片，H5 纵向卡片
 */
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import { Cpu, ChevronRight, Sparkles } from 'lucide-react'
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

  // 状态映射
  const statusInfo: Record<string, { className: string }> = {
    active: { className: 'bg-sage/20 text-sage border-sage/30' },
    inactive: { className: 'bg-muted text-muted-foreground border-border/70' },
  }
  const statusClass = (statusInfo[agent.status] || statusInfo.inactive).className
  const statusLabel = agent.status === 'active' ? tStatus('active') : tStatus('inactive')

  const avatarGradient = getAvatarColor(agent.name)

  if (compact) {
    // 紧凑模式
    return (
      <Link
        href={`/agents/${agent.id}`}
        className="dao-card flex items-center gap-3 sm:gap-4 p-4 group min-w-0"
      >
        {/* 头像 */}
        <div className={`
          shrink-0 w-12 h-12 rounded-xl bg-gradient-to-br ${avatarGradient}
          flex items-center justify-center text-primary-foreground font-serif font-bold text-lg
          shadow-lg
        `}>
          {agent.name.charAt(0)}
        </div>

        {/* 信息 */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 min-w-0">
            <h3 className="font-serif font-semibold text-foreground group-hover:text-gold transition-colors truncate min-w-0">
              {agent.name}
            </h3>
            <span className={`text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap shrink-0 ${statusClass}`}>
              {statusLabel}
            </span>
          </div>
          <p className="text-xs text-muted-foreground mt-0.5 truncate">
            {agent.personality ? truncateText(agent.personality, 40) : t('noPersona')}
          </p>
        </div>

        {/* 模型 */}
        <div className="hidden sm:flex items-center gap-3 text-xs text-muted-foreground shrink-0">
          <span className="flex items-center gap-1 whitespace-nowrap">
            <Cpu className="w-3.5 h-3.5" />
            <span className="truncate max-w-[12ch]">{agent.model_name}</span>
          </span>
        </div>

        <ChevronRight className="w-5 h-5 text-sage group-hover:text-gold transition-colors shrink-0" />
      </Link>
    )
  }

  // 默认模式
  return (
    <Link
      href={`/agents/${agent.id}`}
      className="dao-card flex flex-col p-5 group h-full min-w-0"
    >
      {/* 顶部：头像 + 状态 */}
      <div className="flex items-start justify-between gap-2 mb-4">
        <div className={`
          shrink-0 w-16 h-16 rounded-2xl bg-gradient-to-br ${avatarGradient}
          flex items-center justify-center text-primary-foreground font-serif font-bold text-2xl
          shadow-lg transition-transform duration-300 group-hover:scale-105
        `}>
          {agent.name.charAt(0)}
        </div>
        <div className="flex items-center gap-2 min-w-0">
          {agent.status === 'active' && (
            <Sparkles className="w-4 h-4 text-gold animate-pulse shrink-0" />
          )}
          <span className={`text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap shrink-0 ${statusClass}`}>
            {statusLabel}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <h3 className="font-serif font-bold text-lg text-foreground group-hover:text-gold transition-colors mb-1.5 truncate">
        {agent.name}
      </h3>

      {/* 性格描述 */}
      <p className="text-sm text-muted-foreground line-clamp-3 mb-4 flex-1">
        {agent.personality || t('noPersona')}
      </p>

      {/* 底部分隔 */}
      <div className="dao-divider my-3 text-[10px]">
        <Sparkles className="w-3 h-3" />
      </div>

      {/* 底部信息 */}
      <div className="flex items-center justify-between text-xs text-muted-foreground min-w-0">
        <span className="flex items-center gap-1 min-w-0 truncate">
          <Cpu className="w-3.5 h-3.5 shrink-0" />
          <span className="truncate">{agent.model_name}</span>
        </span>
      </div>
    </Link>
  )
}

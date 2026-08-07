/**
 * 道人卡片组件 - 道教仙人风格
 * 显示头像占位、名称、性格摘要、模型、已服用金丹数
 * 响应式：桌面端网格卡片，H5 纵向卡片
 */
import { Link } from 'react-router-dom'
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
    'from-cinnabar-500 to-cinnabar-700',
    'from-jade-500 to-jade-700',
    'from-gold-500 to-gold-700',
    'from-blue-500 to-blue-700',
    'from-purple-500 to-purple-700',
    'from-teal-500 to-teal-700',
  ]
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return colors[Math.abs(hash) % colors.length]
}

/** 状态映射 */
const STATUS_MAP: Record<string, { label: string; className: string }> = {
  active: { label: '活跃', className: 'bg-jade-500/20 text-jade-300 border-jade-500/30' },
  inactive: { label: '沉睡', className: 'bg-ink-500/30 text-ink-400 border-ink-400/20' },
}

export default function AgentCard({ agent, compact = false }: AgentCardProps) {
  const statusInfo = STATUS_MAP[agent.status] || STATUS_MAP.inactive
  const avatarGradient = getAvatarColor(agent.name)

  if (compact) {
    // 紧凑模式
    return (
      <Link
        to={`/agents/${agent.id}`}
        className="dao-card flex items-center gap-4 p-4 group"
      >
        {/* 头像 */}
        <div className={`
          flex-shrink-0 w-12 h-12 rounded-xl bg-gradient-to-br ${avatarGradient}
          flex items-center justify-center text-white font-serif font-bold text-lg
          shadow-lg
        `}>
          {agent.name.charAt(0)}
        </div>

        {/* 信息 */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-serif font-semibold text-rice-paper-100 group-hover:text-gold-300 transition-colors">
              {agent.name}
            </h3>
            <span className={`text-[10px] px-2 py-0.5 rounded-full border ${statusInfo.className}`}>
              {statusInfo.label}
            </span>
          </div>
          <p className="text-xs text-ink-400 mt-0.5 truncate">
            {agent.personality ? truncateText(agent.personality, 40) : '暂无性格描述'}
          </p>
        </div>

        {/* 模型 */}
        <div className="hidden sm:flex items-center gap-3 text-xs text-ink-400">
          <span className="flex items-center gap-1">
            <Cpu className="w-3.5 h-3.5" />
            {agent.model_name}
          </span>
        </div>

        <ChevronRight className="w-5 h-5 text-ink-500 group-hover:text-gold-400 transition-colors" />
      </Link>
    )
  }

  // 默认模式
  return (
    <Link
      to={`/agents/${agent.id}`}
      className="dao-card flex flex-col p-5 group h-full"
    >
      {/* 顶部：头像 + 状态 */}
      <div className="flex items-start justify-between mb-4">
        <div className={`
          w-16 h-16 rounded-2xl bg-gradient-to-br ${avatarGradient}
          flex items-center justify-center text-white font-serif font-bold text-2xl
          shadow-lg transition-transform duration-300 group-hover:scale-105
        `}>
          {agent.name.charAt(0)}
        </div>
        <div className="flex items-center gap-2">
          {agent.status === 'active' && (
            <Sparkles className="w-4 h-4 text-gold-400 animate-pulse" />
          )}
          <span className={`text-[10px] px-2 py-0.5 rounded-full border ${statusInfo.className}`}>
            {statusInfo.label}
          </span>
        </div>
      </div>

      {/* 名称 */}
      <h3 className="font-serif font-bold text-lg text-rice-paper-100 group-hover:text-gold-300 transition-colors mb-1.5">
        {agent.name}
      </h3>

      {/* 性格描述 */}
      <p className="text-sm text-ink-400 line-clamp-3 mb-4 flex-1">
        {agent.personality || '暂无性格描述'}
      </p>

      {/* 底部分隔 */}
      <div className="dao-divider my-3 text-[10px]">
        <Sparkles className="w-3 h-3" />
      </div>

      {/* 底部信息 */}
      <div className="flex items-center justify-between text-xs text-ink-400">
        <span className="flex items-center gap-1">
          <Cpu className="w-3.5 h-3.5" />
          {agent.model_name}
        </span>
      </div>
    </Link>
  )
}

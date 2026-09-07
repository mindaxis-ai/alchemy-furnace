'use client'

/**
 * 统一头像展示组件：所有头像位置共用。
 * - src 经 normalizeAvatarUrl 校验后才进 <img src>；非法/空值直接渲染 fallback，不渲染 img
 * - 外链图片不发送本地应用 Referer，兼容开启防盗链的头像 CDN
 * - onError 只标记 broken=true，绝不重设同一 src 触发重试循环
 * - broken 重置选 key 方案：图片子组件以 normalized src 为 key，src 变化时整棵子树重挂载，
 *   broken 状态随之归零（effect 方案会被 react-hooks/set-state-in-effect 拦截）
 * - fallback：initial=首字渐变（沿用 agent-card 的 getAvatarColor 逻辑，该函数未导出故内联），
 *   bot=默认机器人图标；fallback 与 img 共用同一容器类，尺寸/shape/边框/aria-label 保持一致
 */
import { useState } from 'react'
import { Bot } from 'lucide-react'
import { normalizeAvatarUrl } from '@/lib/avatar-url'

export interface EntityAvatarProps {
  name: string
  src?: string | null
  size: 'sm' | 'md' | 'lg'
  shape?: 'square' | 'circle'
  fallback?: 'initial' | 'bot'
  alt?: string
}

/** 生成头像渐变颜色（根据名称确定性生成），与 agent-card.tsx 的私有实现保持一致 */
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

const SIZE_CLASS: Record<EntityAvatarProps['size'], string> = {
  sm: 'h-8 w-8',
  md: 'h-12 w-12',
  lg: 'h-16 w-16',
}

const SHAPE_CLASS: Record<NonNullable<EntityAvatarProps['shape']>, string> = {
  square: 'rounded-xl',
  circle: 'rounded-full',
}

interface FallbackProps {
  name: string
  size: EntityAvatarProps['size']
  shape: NonNullable<EntityAvatarProps['shape']>
  fallback: NonNullable<EntityAvatarProps['fallback']>
  label: string
}

function AvatarFallback({ name, size, shape, fallback, label }: FallbackProps) {
  const baseClass = `flex shrink-0 items-center justify-center overflow-hidden ${SIZE_CLASS[size]} ${SHAPE_CLASS[shape]}`
  if (fallback === 'bot') {
    return (
      <div
        aria-label={label}
        className={`${baseClass} bg-muted text-muted-foreground ${SHAPE_CLASS[shape]}`}
      >
        <Bot className="h-1/2 w-1/2" aria-hidden="true" />
      </div>
    )
  }
  return (
    <div
      aria-label={label}
      className={`${baseClass} bg-gradient-to-br font-serif font-bold text-primary-foreground ${getAvatarColor(name)}`}
    >
      {name.charAt(0)}
    </div>
  )
}

/** 图片子组件：key=normalized，src 变化即重挂载，broken 自动重置 */
function AvatarImage({
  normalized,
  label,
  size,
  shape,
  fallback,
  name,
}: {
  normalized: string
  label: string
  size: EntityAvatarProps['size']
  shape: NonNullable<EntityAvatarProps['shape']>
  fallback: NonNullable<EntityAvatarProps['fallback']>
  name: string
}) {
  const [broken, setBroken] = useState(false)
  if (broken) {
    return <AvatarFallback name={name} size={size} shape={shape} fallback={fallback} label={label} />
  }
  return (
    <div className={`flex shrink-0 items-center justify-center overflow-hidden ${SIZE_CLASS[size]} ${SHAPE_CLASS[shape]}`}>
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={normalized}
        alt={label}
        referrerPolicy="no-referrer"
        className={`h-full w-full object-cover ${SHAPE_CLASS[shape]}`}
        onError={() => setBroken(true)}
      />
    </div>
  )
}

export function EntityAvatar({
  name,
  src,
  size,
  shape = 'square',
  fallback = 'initial',
  alt,
}: EntityAvatarProps) {
  const normalized = normalizeAvatarUrl(src)
  const label = alt ?? name
  if (normalized === undefined) {
    return <AvatarFallback name={name} size={size} shape={shape} fallback={fallback} label={label} />
  }
  return (
    <AvatarImage
      key={normalized}
      normalized={normalized}
      label={label}
      size={size}
      shape={shape}
      fallback={fallback}
      name={name}
    />
  )
}

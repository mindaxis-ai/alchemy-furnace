'use client'

/**
 * 通用简介浮窗（飞书式）
 *   - kind='user'  : 展示用户档案(display_name + bio + 跳转设置)
 *   - kind='agent' : 展示道人(name + personality + model_name + 主动性)
 *
 * 不引第三方 popover 库,纯 fixed 定位 + click-outside + 锚元素相对定位。
 * 触发元素(头像)用锚定 anchorRef;Popover 在 body 直接渲染 fixed 层,
 * 避免父容器的 overflow:hidden / transform 把浮窗裁掉。
 */
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useRouter } from 'next/navigation'
import { useTranslations } from 'next-intl'
import { Settings, X, Bot, User, Sparkles, Cpu, Flame } from 'lucide-react'
import type { Agent } from '@/services/types'
import type { UserProfile } from '@/services/userService'

export type ProfileKind = 'user' | 'agent'

export interface ProfilePopoverProps {
  kind: ProfileKind
  /** 锚元素(头像) ref,用来定位浮窗(React 19 下 RefObject<T | null>) */
  anchorRef: React.RefObject<HTMLElement | null>
  /** 触发开关 */
  open: boolean
  /** 关闭回调 */
  onClose: () => void
  /** user 模式必填 */
  userProfile?: UserProfile | null
  /** agent 模式必填 */
  agent?: Agent | null
}

/** 计算位置:默认在锚元素右下方,溢出屏幕则翻转到左/上 */
function computePosition(anchor: HTMLElement): React.CSSProperties {
  const rect = anchor.getBoundingClientRect()
  const popoverW = 320
  const margin = 8
  // 下方放不下则放到上方
  const spaceBelow = window.innerHeight - rect.bottom
  const spaceAbove = rect.top
  const placeAbove = spaceBelow < 220 && spaceAbove > spaceBelow
  const top = placeAbove
    ? Math.max(margin, rect.top - 8) // 实际定位时由 transform 调整
    : rect.bottom + 6

  // 横向:贴锚元素左侧,溢出则左移
  let left = rect.left
  if (left + popoverW + margin > window.innerWidth) {
    left = window.innerWidth - popoverW - margin
  }
  if (left < margin) left = margin

  // 实际 top:上方模式 = 锚元素底 - 浮窗高 - 6
  const finalTop = placeAbove
    ? Math.max(margin, rect.top - 6) // 仅作 fallback,真实计算依赖 content
    : top
  return {
    position: 'fixed',
    top: finalTop,
    left,
    width: popoverW,
    zIndex: 60,
  }
}

export function ProfilePopover({
  kind,
  anchorRef,
  open,
  onClose,
  userProfile,
  agent,
}: ProfilePopoverProps) {
  const router = useRouter()
  const t = useTranslations('profile')
  const tChat = useTranslations('chatMessage')
  const tGroup = useTranslations('groupChat')
  const popRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<React.CSSProperties>({ visibility: 'hidden' })

  // 打开时计算位置;滚动/resize 时重算
  useLayoutEffect(() => {
    if (!open) return
    const recompute = () => {
      const anchor = anchorRef.current
      if (!anchor) return
      const pop = popRef.current
      if (!pop) return
      const style = computePosition(anchor)
      // 如果是 above 模式:finalTop 改为 (anchor.top - popHeight - 6)
      const rect = anchor.getBoundingClientRect()
      const popH = pop.offsetHeight
      const spaceBelow = window.innerHeight - rect.bottom
      const placeAbove = spaceBelow < 220 && rect.top > spaceBelow
      if (placeAbove) {
        style.top = Math.max(8, rect.top - popH - 6)
      }
      setPos(style)
    }
    recompute()
    window.addEventListener('resize', recompute)
    window.addEventListener('scroll', recompute, true)
    return () => {
      window.removeEventListener('resize', recompute)
      window.removeEventListener('scroll', recompute, true)
    }
  }, [open, anchorRef])

  // click outside 关闭
  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      const target = e.target as Node
      if (popRef.current?.contains(target)) return
      if (anchorRef.current?.contains(target)) return
      onClose()
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open, onClose, anchorRef])

  // Esc 关闭
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  if (!open) return null

  const popover = (
    <div
      ref={popRef}
      role="dialog"
      aria-label={kind === 'user' ? t('userTitle') : t('agentTitle')}
      style={pos}
      className="
        rounded-2xl shadow-2xl shadow-black/40
        bg-card border border-gold/30
        animate-in fade-in zoom-in-95 duration-150
        overflow-hidden
      "
      onClick={e => e.stopPropagation()}
    >
      {/* 头部:头像 + 名字 + 关闭 */}
      <div className="flex items-start gap-3 p-4 bg-gradient-to-br from-gold/10 to-transparent">
        {kind === 'user' ? (
          <UserAvatar profile={userProfile} />
        ) : (
          <AgentAvatar agent={agent} />
        )}
        <div className="flex-1 min-w-0">
          {kind === 'user' ? (
            <>
              <p className="text-sm font-serif font-bold text-gold truncate">
                {userProfile?.display_name || t('defaultUser')}
              </p>
              <p className="text-[10px] text-muted-foreground mt-0.5">{t('userSubtitle')}</p>
            </>
          ) : (
            <>
              <p className="text-sm font-serif font-bold text-gold truncate">
                {agent?.name}
              </p>
              <p className="text-[10px] text-muted-foreground mt-0.5 truncate">
                {agent?.model_name}
              </p>
            </>
          )}
        </div>
        <button
          type="button"
          aria-label="关闭"
          onClick={onClose}
          className="p-1 rounded-md hover:bg-secondary text-muted-foreground hover:text-foreground transition-colors shrink-0"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* 简介正文 */}
      <div className="px-4 pb-3">
        {kind === 'user' ? (
          userProfile?.bio ? (
            <p className="text-xs text-foreground/90 leading-relaxed whitespace-pre-wrap break-words max-h-40 overflow-y-auto">
              {userProfile.bio}
            </p>
          ) : (
            <p className="text-xs text-muted-foreground italic">{t('noBio')}</p>
          )
        ) : (
          <>
            {agent?.personality ? (
              <p className="text-xs text-foreground/90 leading-relaxed whitespace-pre-wrap break-words max-h-40 overflow-y-auto">
                {agent.personality}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground italic">{t('noPersonality')}</p>
            )}
            {/* 主动性指标 */}
            {typeof agent?.proactivity === 'number' && (
              <div className="mt-3 flex items-center gap-2 text-[10px] text-muted-foreground">
                <Flame className="w-3 h-3 text-gold shrink-0" />
                <span className="shrink-0">{tGroup('proactivity')}</span>
                <div className="flex-1 h-1 rounded-full bg-muted overflow-hidden">
                  <div
                    className="h-full bg-gradient-to-r from-gold to-primary"
                    style={{ width: `${Math.max(0, Math.min(100, agent.proactivity))}%` }}
                  />
                </div>
                <span className="text-gold font-medium tabular-nums w-7 text-right">{agent.proactivity}</span>
              </div>
            )}
          </>
        )}
      </div>

      {/* 底部:用户模式跳设置 */}
      {kind === 'user' && (
        <div className="border-t border-border/60 px-3 py-2 bg-muted/40">
          <button
            type="button"
            onClick={() => {
              onClose()
              router.push('/settings?tab=profile')
            }}
            className="
              w-full flex items-center justify-center gap-1.5
              py-1.5 rounded-md
              text-xs text-gold hover:text-foreground
              hover:bg-gold/10 transition-colors
            "
          >
            <Settings className="w-3.5 h-3.5" />
            <span>{t('editInSettings')}</span>
          </button>
        </div>
      )}
    </div>
  )

  return typeof document !== 'undefined' ? createPortal(popover, document.body) : popover
}

/* ========== 内部小组件 ========== */

function UserAvatar({ profile }: { profile?: UserProfile | null }) {
  const t = useTranslations('profile')
  const initial = (profile?.display_name || t('defaultUser')).charAt(0)
  if (profile?.avatar) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={profile.avatar}
        alt={profile.display_name}
        className="w-12 h-12 rounded-full object-cover border-2 border-gold/40 shrink-0"
      />
    )
  }
  return (
    <div className="w-12 h-12 rounded-full bg-gradient-to-br from-primary/30 to-primary/10 border-2 border-primary/30 flex items-center justify-center text-primary font-serif font-bold shrink-0">
      {initial}
    </div>
  )
}

function AgentAvatar({ agent }: { agent?: Agent | null }) {
  if (!agent) {
    return (
      <div className="w-12 h-12 rounded-full bg-sage/15 border-2 border-sage/30 flex items-center justify-center text-sage shrink-0">
        <Bot className="w-6 h-6" />
      </div>
    )
  }
  if (agent.avatar) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={agent.avatar}
        alt={agent.name}
        className="w-12 h-12 rounded-full object-cover border-2 border-gold/40 shrink-0"
      />
    )
  }
  return (
    <div className="w-12 h-12 rounded-full bg-gradient-to-br from-sage to-sage/70 border-2 border-gold/40 flex items-center justify-center text-white font-serif font-bold shrink-0">
      {agent.name.charAt(0)}
    </div>
  )
}

'use client'

import type { ReactNode, CSSProperties } from 'react'
import { cn } from '@/lib/utils'

/**
 * A floating frosted-warm glass display case.
 * Gentle infinite vertical float; on hover it lifts higher,
 * the shadow deepens, and a faint cinnabar glow rises from below.
 */
export function FloatCard({
  children,
  className,
  float = true,
  delay = 0,
  style,
}: {
  children: ReactNode
  className?: string
  float?: boolean
  delay?: number
  style?: CSSProperties
}) {
  return (
    <div
      className={cn(float && 'float-soft', 'group/float')}
      style={{ animationDelay: `${delay}s`, ...style }}
    >
      <div
        className={cn(
          'relative overflow-hidden rounded-[20px] border border-border/60',
          'bg-card/80 backdrop-blur-sm',
          'shadow-[0_25px_50px_-12px_rgba(60,40,20,0.08)]',
          'transition-[transform,box-shadow] duration-700 ease-out',
          'group-hover/float:-translate-y-2',
          'group-hover/float:shadow-[0_40px_70px_-15px_rgba(60,40,20,0.16)]',
          className,
        )}
      >
        {/* cinnabar glow rising from below on hover */}
        <div
          aria-hidden
          className="pointer-events-none absolute inset-x-0 -bottom-16 h-32 opacity-0 blur-2xl transition-opacity duration-700 group-hover/float:opacity-100"
          style={{
            background:
              'radial-gradient(60% 100% at 50% 100%, rgba(181,74,63,0.28), transparent 70%)',
          }}
        />
        {children}
      </div>
    </div>
  )
}

/** Ancient-coin circular icon container — round with a soft square well. */
export function CoinIcon({
  children,
  className,
  tone = 'gold',
}: {
  children: ReactNode
  className?: string
  tone?: 'gold' | 'cinnabar' | 'sage'
}) {
  const tones = {
    gold: 'text-gold ring-gold/25 bg-gold/10',
    cinnabar: 'text-primary ring-primary/25 bg-primary/10',
    sage: 'text-sage ring-sage/25 bg-sage/10',
  }
  return (
    <span
      className={cn(
        'relative grid size-11 shrink-0 place-items-center rounded-full ring-1',
        tones[tone],
        className,
      )}
    >
      {/* inner square well, like a coin's center hole */}
      <span className="absolute size-3.5 rounded-[3px] border border-current/20" aria-hidden />
      <span className="relative">{children}</span>
    </span>
  )
}

/** Small vermilion seal-stamp dot used as a quiet oriental accent. */
export function SealDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden
      className={cn('inline-block size-1.5 rounded-full bg-primary/70', className)}
    />
  )
}

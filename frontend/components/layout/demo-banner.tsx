'use client'

/**
 * 演示模式提示(007-demo-mode)
 *
 * 客户端组件:挂载时探测 /api/v1/system/health 的 mode 字段,为 'demo' 时
 * 右下角显示悬浮卡片。生产 output:'export' 下无法在构建期得知运行模式,
 * 故延迟到浏览器运行期探测;非演示模式或已关闭时不渲染任何内容。
 *
 * 关闭状态存 sessionStorage:同一会话内(含 SPA 跨 layout 跳转、组件重挂载)
 * 不再显示;关闭浏览器标签页后重新打开才会再次出现。
 *
 * 视觉:宣纸底色(纸纹颗粒 + 暖白底) + 朱砂火焰纹(右下角火纹水印 +
 * 印章式火焰图标),与 dao-card / FloatCard 同一套暖调美学。
 */

import { useEffect, useState } from 'react'
import { useTranslations } from 'next-intl'
import { Flame, X } from 'lucide-react'
import { getMode } from '@/lib/demo-mode'

const DISMISS_KEY = 'demo-banner-dismissed'

/** 宣纸颗粒纹理 — 与 body 背景同款的 noise SVG(data URI) */
const PAPER_GRAIN =
  "url(\"data:image/svg+xml,%3Csvg viewBox='0 0 240 240' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/%3E%3CfeColorMatrix type='saturate' values='0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.04'/%3E%3C/svg%3E\")"

/** lucide flame 的 path(24×24 viewBox),放大作低透明度火纹水印 */
const FLAME_PATH =
  'M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z'

/** 右下角火纹水印:一大一小两缕火焰,朱砂/陈金极低透明度 */
function FlameWatermark() {
  return (
    <>
      <svg
        aria-hidden
        viewBox="0 0 24 24"
        fill="currentColor"
        className="pointer-events-none absolute -bottom-5 -right-4 size-24 rotate-12 text-primary opacity-[0.07]"
      >
        <path d={FLAME_PATH} />
      </svg>
      <svg
        aria-hidden
        viewBox="0 0 24 24"
        fill="currentColor"
        className="pointer-events-none absolute -bottom-2 right-14 size-10 -rotate-6 text-gold opacity-[0.09]"
      >
        <path d={FLAME_PATH} />
      </svg>
    </>
  )
}

export function DemoBanner() {
  const t = useTranslations('demoBanner')
  // 默认不显示,挂载后确认是 demo 且未关闭过才展示(避免 SSR/首屏闪烁)
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (sessionStorage.getItem(DISMISS_KEY) === '1') return
    getMode().then((m) => {
      if (m === 'demo') setVisible(true)
    })
  }, [])

  if (!visible) return null

  const dismiss = () => {
    sessionStorage.setItem(DISMISS_KEY, '1')
    setVisible(false)
  }

  return (
    <div
      className="fixed bottom-4 right-4 z-50 flex max-w-xs items-center gap-3 overflow-hidden rounded-2xl border border-gold/30 px-4 py-3 shadow-[0_25px_50px_-12px_rgba(60,40,20,0.15)] backdrop-blur-sm animate-in fade-in slide-in-from-bottom-2 duration-300"
      style={{
        backgroundColor: 'color-mix(in srgb, var(--paper) 96%, transparent)',
        backgroundImage: PAPER_GRAIN,
      }}
    >
      {/* 顶部一缕金线(印章封条感) */}
      <div
        aria-hidden
        className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-gold/50 to-transparent"
      />
      <FlameWatermark />

      {/* 印章式火焰图标(铜钱圆环 + 朱砂火) */}
      <span className="relative grid size-9 shrink-0 place-items-center rounded-full bg-primary/10 ring-1 ring-primary/25">
        <Flame className="size-4 text-primary" />
      </span>

      <div className="relative min-w-0 flex-1">
        <p className="font-serif text-[13px] font-bold tracking-wide text-primary">
          {t('title')}
        </p>
        <p className="mt-0.5 text-xs leading-relaxed text-sage">{t('text')}</p>
      </div>

      <button
        type="button"
        onClick={dismiss}
        aria-label={t('dismiss')}
        className="relative -mr-1 shrink-0 rounded-full p-1 text-sage/70 transition-colors hover:bg-gold/10 hover:text-foreground"
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}

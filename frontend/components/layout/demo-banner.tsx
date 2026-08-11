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
 */

import { useEffect, useState } from 'react'
import { useTranslations } from 'next-intl'
import { FlaskConical, X } from 'lucide-react'
import { getMode } from '@/lib/demo-mode'

const DISMISS_KEY = 'demo-banner-dismissed'

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
    <div className="fixed bottom-4 right-4 z-50 flex max-w-xs items-center gap-2.5 rounded-xl border border-amber-200/80 bg-amber-50/95 px-3.5 py-2.5 shadow-[0_12px_32px_-12px_rgba(180,120,30,0.35)] backdrop-blur-sm animate-in fade-in slide-in-from-bottom-2 duration-300 dark:border-amber-800/60 dark:bg-amber-950/80">
      <FlaskConical className="h-4 w-4 shrink-0 text-amber-700 dark:text-amber-300" />
      <span className="flex-1 text-xs leading-relaxed text-amber-800 dark:text-amber-200">
        {t('text')}
      </span>
      <button
        type="button"
        onClick={dismiss}
        aria-label={t('dismiss')}
        className="shrink-0 rounded p-1 text-amber-700/70 transition-colors hover:bg-amber-200/60 hover:text-amber-900 dark:text-amber-300/70 dark:hover:bg-amber-800/50 dark:hover:text-amber-100"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

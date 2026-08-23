'use client'

import { useState } from 'react'
import { useLocale, useTranslations } from 'next-intl'
import { BrainCircuit, Check, ExternalLink, Loader2, Search } from 'lucide-react'
import { distillNuwa } from '@/services/distillationService'
import type { DistillationDraft } from '@/services/types'

/**
 * 女娲智能蒸馏面板
 * 走正式网络/模型链路(distillNuwa),不加入任何 Mock fallback。
 * 蒸馏完成后只展示候选草稿与来源,只有用户显式「应用」才经 onApply 写入外层表单。
 */
export function NuwaDistillPanel({
  onApply,
}: {
  onApply: (draft: DistillationDraft) => void
}) {
  const t = useTranslations('distillation')
  const locale = useLocale() === 'en' ? 'en' : 'zh-CN'
  const [subject, setSubject] = useState('')
  const [brief, setBrief] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [draft, setDraft] = useState<DistillationDraft | null>(null)

  const run = async () => {
    if (subject.trim().length < 2 || brief.trim().length < 4) return
    setLoading(true)
    setError(null)
    try {
      const result = await distillNuwa({ subject: subject.trim(), brief: brief.trim(), locale })
      // 只生成候选预览,不自动应用
      setDraft(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('error'))
    } finally {
      setLoading(false)
    }
  }

  /** 用户显式确认后才把候选写入外层表单 */
  const apply = () => {
    if (!draft) return
    onApply(draft)
    setDraft(null)
  }

  return (
    <section className="rounded-2xl border border-gold/35 bg-gold/5 p-4">
      <div className="mb-3 flex items-start gap-2">
        <BrainCircuit className="mt-0.5 h-4 w-4 shrink-0 text-gold" />
        <div className="min-w-0">
          <h3 className="font-serif text-sm font-bold text-foreground">{t('title')}</h3>
          <p className="text-[11px] leading-relaxed text-muted-foreground">{t('description')}</p>
        </div>
      </div>

      <div className="space-y-3">
        <div>
          <label className="dao-label">{t('subjectLabel')}</label>
          <input
            value={subject}
            onChange={(event) => setSubject(event.target.value)}
            placeholder={t('subjectPlaceholder')}
            className="dao-input"
          />
        </div>
        <div>
          <label className="dao-label">{t('briefLabel')}</label>
          <textarea
            value={brief}
            onChange={(event) => setBrief(event.target.value)}
            placeholder={t('briefPlaceholder')}
            rows={3}
            className="dao-textarea min-h-20"
          />
        </div>
        <button
          type="button"
          onClick={run}
          disabled={loading || subject.trim().length < 2 || brief.trim().length < 4}
          className="dao-btn-gold w-full disabled:opacity-50"
        >
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Search className="h-4 w-4" />}
          {loading ? t('researching') : t('start')}
        </button>
      </div>

      {loading && (
        <div className="mt-3 grid grid-cols-3 gap-1 text-center text-[10px] text-sage">
          <span>{t('stages.research')}</span>
          <span>{t('stages.verify')}</span>
          <span>{t('stages.distill')}</span>
        </div>
      )}
      {error && <p className="mt-3 break-words text-xs text-primary">{error}</p>}

      {draft && !loading && (
        <div className="mt-3 border-t border-gold/20 pt-3">
          <p className="flex items-center gap-1 text-xs font-medium text-sage">
            <Check className="h-3.5 w-3.5" />
            {t('draftReady', { count: draft.sources.length })}
          </p>

          {/* 候选草稿预览 */}
          <div className="mt-2 rounded-lg border border-gold/20 bg-muted/40 p-3">
            <p className="text-[10px] uppercase tracking-wide text-muted-foreground">
              {t('previewTitle')}
            </p>
            <p className="mt-1 font-serif text-sm font-bold text-foreground">{draft.name}</p>
            {draft.description && (
              <p className="mt-1 line-clamp-3 text-xs leading-relaxed text-muted-foreground">
                {draft.description}
              </p>
            )}
            <div className="mt-2 flex flex-wrap gap-1">
              {draft.tags.slice(0, 5).map((tag) => (
                <span
                  key={tag}
                  className="rounded-full border border-gold/20 px-2 py-0.5 text-[10px] text-gold"
                >
                  {tag}
                </span>
              ))}
            </div>
          </div>

          {/* 资料来源 */}
          <details className="mt-2 text-[11px] text-muted-foreground">
            <summary className="cursor-pointer select-none">{t('sources')}</summary>
            <ul className="mt-1 space-y-1">
              {draft.sources.slice(0, 6).map((source) => (
                <li key={source.url}>
                  <a
                    href={source.url}
                    target="_blank"
                    rel="noreferrer"
                    className="flex min-w-0 items-center gap-1 hover:text-gold"
                  >
                    <ExternalLink className="h-3 w-3 shrink-0" />
                    <span className="truncate">{source.title}</span>
                  </a>
                </li>
              ))}
            </ul>
          </details>

          {/* 显式确认/丢弃 */}
          <div className="mt-3 flex gap-2">
            <button type="button" onClick={apply} className="dao-btn-primary flex-1 text-xs">
              <Check className="h-3.5 w-3.5" />
              {t('apply')}
            </button>
            <button type="button" onClick={() => setDraft(null)} className="dao-btn-ghost text-xs">
              {t('discard')}
            </button>
          </div>
        </div>
      )}
    </section>
  )
}

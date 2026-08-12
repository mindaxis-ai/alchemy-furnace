'use client'

/**
 * 融合炉页面 - 投丹入炉,合而新之
 * - 左 dao-card: 金丹池(搜索 + 卡片网格,点击 toggle 加入融合槽)
 * - 右 dao-card: 融合槽(已选卡片可移除) + 炉火动画 + [开始融合]
 * - 融合中: 卡片飞入炉中 + 火焰爆燃(forceIntensity=1)
 * - 完成后: 弹出 FusionPreviewModal(算子徽标/血统/可编辑/换一炉/保存入库)
 */
import { useEffect, useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useTranslations } from 'next-intl'
import { FlaskConical, X, Search, Loader2, AlertCircle, AlertTriangle } from 'lucide-react'
import { listPills, createPill } from '@/services/pillService'
import { fusePills, withLineage, type FuseResult } from '@/services/fusionService'
import { listProviders } from '@/services/modelService'
import type { Pill } from '@/services/types'
import { BaguaFurnace } from '@/components/alchemy/bagua-furnace'
import type { FurnaceWindow } from '@/components/alchemy/bagua-furnace-fire'
import { FusionPreviewModal } from '@/components/fusion/fusion-preview-modal'

const FURNACE_WINDOWS: FurnaceWindow[] = [
  { id: 'left',   x: 37.35, width: 7.71, top: 50.10, height: 9.08, phase: 0.00 },
  { id: 'center', x: 50.29, width: 8.79, top: 50.49, height: 8.98, phase: 0.45 },
  { id: 'right',  x: 63.18, width: 7.81, top: 50.20, height: 9.08, phase: 0.90 },
]

export default function FusionPage() {
  const t = useTranslations('fusion')
  const router = useRouter()
  const [pool, setPool] = useState<Pill[]>([])
  const [keyword, setKeyword] = useState('')
  const [selected, setSelected] = useState<Pill[]>([])
  const [fusing, setFusing] = useState(false)
  const [result, setResult] = useState<FuseResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [hasProvider, setHasProvider] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    listPills({ page_size: 100 })
      .then((d) => { if (!cancelled) setPool(d.list || []) })
      .catch(() => {})
    listProviders({ page_size: 100 })
      .then((d) => { if (!cancelled) setHasProvider((d.list || []).length > 0) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  const selectedIds = useMemo(() => new Set(selected.map((p) => p.id)), [selected])
  const filteredPool = useMemo(
    () => pool.filter((p) => !keyword || p.name.includes(keyword)),
    [pool, keyword],
  )
  const canFuse = selected.length >= 2 && !fusing && hasProvider

  const toggle = (pill: Pill) => {
    if (selectedIds.has(pill.id)) setSelected((s) => s.filter((p) => p.id !== pill.id))
    else setSelected((s) => [...s, pill])
  }

  const doFuse = async (excludeOperatorId?: string) => {
    if (selected.length < 2) return
    setFusing(true)
    setError(null)
    try {
      const r = await fusePills(selected.map((p) => p.id), excludeOperatorId)
      setResult(r)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setFusing(false)
    }
  }

  const handleSave = async (
    edited: { name: string; description: string },
    goEdit: boolean,
  ) => {
    if (!result || saving) return
    setSaving(true)
    setError(null)
    try {
      const schema = withLineage(result.skill_schema, selected, result.operator)
      const created = await createPill({
        name: edited.name,
        description: edited.description,
        skill_schema: schema,
        tags: ['融合'],
        author: '融合炉',
        version: '1.0.0',
      })
      if (goEdit) {
        router.push(`/pills/${created.id}`)
      } else {
        setResult(null)
        setSelected([])
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="mx-auto max-w-6xl px-4 sm:px-6 py-6 pb-24">
      <div className="mb-6 flex items-start gap-3">
        <FlaskConical className="w-7 h-7 text-gold shrink-0 mt-0.5" strokeWidth={1.75} />
        <div className="min-w-0 flex-1">
          <h1 className="font-serif text-2xl font-black text-foreground sm:text-3xl">
            {t('title')}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('subtitle')}</p>
        </div>
      </div>

      {!hasProvider && (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
          <span>{t('preview.noProvider')}</span>
        </div>
      )}

      {error && (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-xs text-primary">
          <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
          <span className="flex-1">{error}</span>
          <button
            onClick={() => doFuse()}
            className="text-xs font-medium underline-offset-2 hover:underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_1fr]">
        {/* 左: 金丹池 */}
        <div className="dao-card p-5">
          <div className="mb-4 flex items-baseline justify-between gap-3">
            <h2 className="font-serif text-lg font-bold text-foreground">
              {t('poolTitle')}
            </h2>
            <span className="text-xs text-sage">
              {filteredPool.length} / {pool.length}
            </span>
          </div>

          <div className="relative mb-4">
            <Search className="pointer-events-none absolute left-3 top-1/2 w-4 h-4 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder={t('poolSearchPlaceholder')}
              className="dao-input pl-9"
            />
          </div>

          <div className="max-h-[480px] overflow-y-auto pr-1">
            {filteredPool.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">—</p>
            ) : (
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {filteredPool.map((p) => {
                  const sel = selectedIds.has(p.id)
                  return (
                    <button
                      key={p.id}
                      onClick={() => toggle(p)}
                      disabled={fusing}
                      className={`
                        group relative flex flex-col items-center gap-1 rounded-xl border p-2.5 text-center transition-all min-w-0
                        ${sel
                          ? 'border-gold/60 bg-gold/10 ring-1 ring-gold/30'
                          : 'border-border/70 bg-secondary/40 hover:border-gold/30 hover:bg-gold/5'
                        }
                        disabled:opacity-50
                      `}
                    >
                      <div className="w-9 h-9 rounded-lg bg-gradient-to-br from-sage to-sage/70 text-primary-foreground font-serif font-bold flex items-center justify-center shrink-0">
                        {p.name.charAt(0)}
                      </div>
                      <p className="w-full truncate text-xs font-medium text-foreground">
                        {p.name}
                      </p>
                      {p.is_builtin && (
                        <span className="text-[9px] text-sage">builtin</span>
                      )}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        </div>

        {/* 右: 融合槽 + 炉火动画 + 开始按钮 */}
        <div className="dao-card p-5">
          <div className="mb-4 flex items-baseline justify-between gap-3">
            <h2 className="font-serif text-lg font-bold text-foreground">
              {t('slotTitle')}
            </h2>
            <span className="text-xs text-sage">
              {selected.length} / —
            </span>
          </div>

          {selected.length === 0 ? (
            <p className="mb-4 text-xs text-muted-foreground">{t('slotEmpty')}</p>
          ) : (
            <div className="mb-4 flex flex-wrap gap-2">
              {selected.map((p) => (
                <span
                  key={p.id}
                  className={`
                    inline-flex items-center gap-1.5 rounded-full border border-gold/30 bg-gold/10 px-2.5 py-1 text-xs
                    ${fusing ? 'scale-50 opacity-0 translate-y-24 transition-all duration-500' : 'transition-all duration-200'}
                  `}
                >
                  {p.name}
                  {!fusing && (
                    <button
                      onClick={() => toggle(p)}
                      className="text-gold/70 hover:text-gold"
                      aria-label={`remove ${p.name}`}
                    >
                      <X className="w-3 h-3" />
                    </button>
                  )}
                </span>
              ))}
            </div>
          )}

          <div className="relative mx-auto w-56 aspect-square sm:w-64">
            <BaguaFurnace
              alt={t('title')}
              windows={FURNACE_WINDOWS}
              forceIntensity={fusing ? 1 : 0.35}
            />
          </div>

          <button
            onClick={() => doFuse()}
            disabled={!canFuse}
            className="dao-btn-primary mt-5 w-full"
          >
            {fusing ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                {t('fusing')}
              </>
            ) : (
              <>
                <FlaskConical className="w-4 h-4" />
                {t('fuseCta')}
              </>
            )}
          </button>
          {selected.length < 2 && (
            <p className="mt-2 text-center text-[10px] text-sage">{t('fuseMinHint')}</p>
          )}
        </div>
      </div>

      {result && (
        <FusionPreviewModal
          result={result}
          parents={selected}
          saving={saving}
          onReroll={() => doFuse(result.operator.id)}
          onSave={(edited) => handleSave(edited, false)}
          onEdit={(edited) => handleSave(edited, true)}
          onClose={() => setResult(null)}
        />
      )}
    </div>
  )
}

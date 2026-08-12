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
import { FlaskConical, X, Search, Loader2, AlertCircle, AlertTriangle, Check } from 'lucide-react'
import { listPills, createPill } from '@/services/pillService'
import { fusePills, withLineage, type FuseResult } from '@/services/fusionService'
import { listProviders, listModels, updateModel } from '@/services/modelService'
import type { Provider, LLMModel } from '@/services/modelService'
import { getSystemConfig, type FusionModelInfo } from '@/services/systemService'
import type { Pill } from '@/services/types'
import { BaguaFurnace } from '@/components/alchemy/bagua-furnace'
import type { FurnaceWindow } from '@/components/alchemy/bagua-furnace-fire'
import { FusionPreviewModal } from '@/components/fusion/fusion-preview-modal'

const FURNACE_WINDOWS: FurnaceWindow[] = [
  { id: 'left',   x: 37.35, width: 7.71, top: 50.10, height: 9.08, phase: 0.00 },
  { id: 'center', x: 50.29, width: 8.79, top: 50.49, height: 8.98, phase: 0.45 },
  { id: 'right',  x: 63.18, width: 7.81, top: 50.20, height: 9.08, phase: 0.90 },
]

/** 选择器中:供应商 + 其下启用模型 */
interface PickerProvider {
  provider: Provider
  models: LLMModel[]
}

export default function FusionPage() {
  const t = useTranslations('fusion')
  const router = useRouter()
  const [pool, setPool] = useState<Pill[]>([])
  const [keyword, setKeyword] = useState('')
  const [selected, setSelected] = useState<Pill[]>([])
  const [fusing, setFusing] = useState(false)
  const [result, setResult] = useState<FuseResult | null>(null)
  const [errorModal, setErrorModal] = useState<string | null>(null)
  const [hasProvider, setHasProvider] = useState(true)
  const [saving, setSaving] = useState(false)
  const [fusionModel, setFusionModel] = useState<FusionModelInfo | null>(null)

  // 融合模型选择器弹窗
  const [showPicker, setShowPicker] = useState(false)
  const [pickerProviders, setPickerProviders] = useState<PickerProvider[]>([])
  const [pickerLoading, setPickerLoading] = useState(false)
  const [switchingId, setSwitchingId] = useState<string | null>(null)

  /** 重新拉取 banner 用的当前融合模型 */
  const refreshFusionModel = async () => {
    try {
      const c = await getSystemConfig()
      setFusionModel(c.fusion_model_info)
    } catch {}
  }

  useEffect(() => {
    let cancelled = false
    listPills({ page_size: 100 })
      .then((d) => { if (!cancelled) setPool(d.list || []) })
      .catch(() => {})
    listProviders({ page_size: 100 })
      .then((d) => { if (!cancelled) setHasProvider((d.list || []).length > 0) })
      .catch(() => {})
    refreshFusionModel()
    return () => { cancelled = true }
  }, [])

  /** 加载选择器中展示的所有启用供应商 + 启用模型 */
  const loadPickerModels = async () => {
    setPickerLoading(true)
    try {
      const providers = (await listProviders({ page_size: 100 })).list || []
      const enabledProviders = providers.filter(p => p.is_enabled)
      const results = await Promise.all(
        enabledProviders.map(async (provider) => {
          const models = (await listModels(provider.id)) || []
          return {
            provider,
            models: models.filter(m => m.is_enabled),
          }
        }),
      )
      setPickerProviders(results.filter(r => r.models.length > 0))
    } catch {
      setPickerProviders([])
    } finally {
      setPickerLoading(false)
    }
  }

  /** 打开选择器:加载数据 */
  const openPicker = () => {
    setShowPicker(true)
    loadPickerModels()
  }

  /** 选择一个模型作为融合专用:后端事务内自动清除其他记录的 is_fusion */
  const chooseFusionModel = async (model: LLMModel) => {
    if (switchingId) return
    setSwitchingId(model.id)
    try {
      await updateModel(model.id, { is_fusion: true })
      await refreshFusionModel()
      setShowPicker(false)
    } catch (e) {
      setErrorModal(e instanceof Error ? e.message : String(e))
    } finally {
      setSwitchingId(null)
    }
  }

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
    setErrorModal(null)
    try {
      const r = await fusePills(selected.map((p) => p.id), excludeOperatorId)
      setResult(r)
    } catch (e) {
      setErrorModal(e instanceof Error ? e.message : String(e))
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
    setErrorModal(null)
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
      setErrorModal(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  const isFusionConfigError = (msg: string) =>
    msg.includes('金丹融合') ||
    msg.toLowerCase().includes('pill fusion') ||
    msg.includes('融合专用模型') ||
    msg.includes('fusion model')

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

      {/* 当前融合模型 banner:已配置显示模型名,未配置显示醒目警告 */}
      <div className={`mb-4 flex items-start gap-2 rounded-lg border px-3 py-2 text-xs ${
        fusionModel?.configured
          ? 'border-sage/30 bg-sage/5 text-sage'
          : 'border-primary/30 bg-primary/5 text-primary'
      }`}>
        {fusionModel?.configured ? (
          <Check className="w-4 h-4 mt-0.5 shrink-0" />
        ) : (
          <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
        )}
        <div className="flex-1 min-w-0">
          {fusionModel?.configured ? (
            <span className="break-words">
              {t('preview.fusionModelLabel')}:
              <span className="font-medium ml-1">
                {fusionModel.model_display_name || fusionModel.model_name}
              </span>
              <span className="ml-1 text-sage/70">
                ({fusionModel.provider_display_name || fusionModel.provider_name})
              </span>
            </span>
          ) : (
            <span className="break-words">{t('preview.noFusionModel')}</span>
          )}
        </div>
        <button
          onClick={openPicker}
          className="inline-flex items-center gap-1 text-xs font-medium underline-offset-2 hover:underline whitespace-nowrap"
        >
          {fusionModel?.configured ? t('preview.changeCta') : t('preview.goToSettingsCta')}
        </button>
      </div>

      {!hasProvider && (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
          <span className="flex-1">{t('preview.noProvider')}</span>
          <button
            onClick={() => router.push('/settings')}
            className="text-xs font-medium underline-offset-2 hover:underline whitespace-nowrap"
          >
            {t('preview.goToSettingsCta')}
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

      {/* 融合模型选择弹窗(内嵌,不跳设置) */}
      {showPicker && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={() => !switchingId && setShowPicker(false)}
        >
          <div
            className="dao-card w-full max-w-lg max-h-[80vh] flex flex-col p-5"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-start justify-between gap-3 mb-1">
              <h2 className="font-serif text-lg font-bold text-foreground">
                {t('preview.picker.title')}
              </h2>
              <button
                onClick={() => setShowPicker(false)}
                disabled={!!switchingId}
                className="text-muted-foreground hover:text-foreground disabled:opacity-30"
                aria-label="close"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <p className="mb-4 text-xs text-muted-foreground">
              {t('preview.picker.subtitle')}
            </p>

            <div className="flex-1 overflow-y-auto -mx-1 px-1">
              {pickerLoading ? (
                <div className="flex items-center justify-center py-12 text-muted-foreground">
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  {t('preview.picker.loading')}
                </div>
              ) : pickerProviders.length === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  <p className="font-medium text-foreground mb-1">
                    {t('preview.picker.emptyTitle')}
                  </p>
                  <p>{t('preview.picker.emptyDesc')}</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {pickerProviders.map(({ provider, models }) => (
                    <div key={provider.id}>
                      <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-wider text-sage">
                        {provider.display_name || provider.name}
                      </h3>
                      <ul className="space-y-1.5">
                        {models.map((m) => {
                          const isCurrent = m.is_fusion
                          const switching = switchingId === m.id
                          return (
                            <li key={m.id}>
                              <button
                                onClick={() => chooseFusionModel(m)}
                                disabled={!!switchingId}
                                className={`
                                  group flex w-full items-center justify-between gap-3 rounded-lg border px-3 py-2 text-left transition-all
                                  ${isCurrent
                                    ? 'border-gold/50 bg-gold/10'
                                    : 'border-border/70 bg-secondary/40 hover:border-gold/30 hover:bg-gold/5'
                                  }
                                  disabled:opacity-60
                                `}
                              >
                                <div className="min-w-0 flex-1">
                                  <p className="truncate text-sm font-medium text-foreground">
                                    {m.display_name || m.name}
                                  </p>
                                  <p className="truncate text-[10px] text-muted-foreground">
                                    {m.name}
                                  </p>
                                </div>
                                {switching ? (
                                  <Loader2 className="w-3.5 h-3.5 animate-spin text-gold" />
                                ) : isCurrent ? (
                                  <span className="inline-flex items-center gap-1 text-[10px] font-medium text-gold">
                                    <Check className="w-3 h-3" />
                                    {t('preview.picker.currentBadge')}
                                  </span>
                                ) : (
                                  <span className="text-[10px] font-medium text-muted-foreground group-hover:text-gold">
                                    {t('preview.picker.setCta')}
                                  </span>
                                )}
                              </button>
                            </li>
                          )
                        })}
                      </ul>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 错误弹窗:融合失败/保存失败统一展示,提供重试或关闭 */}
      {errorModal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
          onClick={() => setErrorModal(null)}
        >
          <div
            className="dao-card w-full max-w-md p-5"
            onClick={(e) => e.stopPropagation()}
            role="alertdialog"
            aria-modal="true"
          >
            <div className="flex items-start gap-3 mb-3">
              <div className="shrink-0 w-9 h-9 rounded-full bg-primary/10 text-primary flex items-center justify-center">
                <AlertCircle className="w-5 h-5" />
              </div>
              <div className="min-w-0 flex-1">
                <h2 className="font-serif text-lg font-bold text-foreground">
                  {t('preview.errorTitle')}
                </h2>
                <p className="mt-2 text-sm text-foreground/90 break-words whitespace-pre-wrap leading-relaxed">
                  {errorModal}
                </p>
              </div>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button
                onClick={() => setErrorModal(null)}
                className="dao-btn-ghost text-sm"
              >
                {t('preview.errorClose')}
              </button>
              {isFusionConfigError(errorModal) ? (
                <button
                  onClick={() => { setErrorModal(null); router.push('/settings') }}
                  className="dao-btn-primary text-sm"
                >
                  {t('preview.goToSettingsCta')}
                </button>
              ) : (
                <button
                  onClick={() => { setErrorModal(null); doFuse() }}
                  disabled={fusing}
                  className="dao-btn-primary text-sm"
                >
                  {t('preview.errorRetry')}
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

'use client'

/**
 * 金丹阁页面 - 语言模式技能包管理
 * 金丹列表卡片（道教丹药风格）
 * 关键词搜索 + 内置过滤 + 快捷赠予道人 + 炼制新金丹
 * H5 优化: 单列卡片布局
 */
import { useState, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { useTranslations } from 'next-intl'
import {
  Plus,
  Search,
  CircleDot,
  Loader2,
  FlaskConical,
  X,
  AlertCircle,
  RefreshCw,
} from 'lucide-react'
import { usePill } from '@/contexts/PillContext'
import { PillCard } from '@/components/pill-card'
import { BindAgentModal } from '@/components/bind-agent-modal'
import { TopTabs } from '@/components/interaction/top-tabs'
import { NuwaDistillPanel } from '@/components/nuwa-distill-panel'
import { emptySkillSchema } from '@/services/pillService'
import type { DistillationDraft, Pill } from '@/services/types'

/** 内置过滤选项 */
type BuiltinFilter = 'all' | 'builtin' | 'custom'

export default function PillsPage() {
  const t = useTranslations('pills')
  const router = useRouter()
  const { state, fetchPills, addPill } = usePill()
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [builtinFilter, setBuiltinFilter] = useState<BuiltinFilter>('all')
  const [bindingPill, setBindingPill] = useState<Pill | null>(null)
  const [creating, setCreating] = useState(false)
  const [distilledDraft, setDistilledDraft] = useState<DistillationDraft | null>(null)

  /** 按搜索条件加载金丹列表 */
  const loadPills = useCallback((keyword: string, filter: BuiltinFilter) => {
    fetchPills({
      keyword: keyword.trim() || undefined,
      is_builtin: filter === 'all' ? undefined : filter === 'builtin',
    })
  }, [fetchPills])

  // 首次立即加载；输入搜索词时防抖。保持单一请求入口，避免初始化双请求竞态。
  useEffect(() => {
    const timer = setTimeout(
      () => loadPills(searchQuery, builtinFilter),
      searchQuery.trim() ? 300 : 0,
    )
    return () => clearTimeout(timer)
  }, [searchQuery, builtinFilter, loadPills])

  /** 创建金丹（携带空 skill_schema，创建后进入编辑器完善） */
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setCreating(true)
    const pill = await addPill({
      name: name.trim(),
      description: description.trim() || undefined,
      skill_schema: distilledDraft?.skill_schema || emptySkillSchema(),
      tags: distilledDraft?.tags || [],
      version: '1.0.0',
    })
    setCreating(false)
    if (pill) {
      setShowCreate(false)
      setName('')
      setDescription('')
      setDistilledDraft(null)
      router.push(`/pills/${pill.id}`)
    }
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-3">
            <CircleDot className="w-6 h-6 text-gold" />
            <h1 className="page-title">{t('title')}</h1>
          </div>
          <p className="page-subtitle">{t('subtitle')}</p>
        </div>

        <button
          onClick={() => setShowCreate(true)}
          className="dao-btn-primary self-start whitespace-nowrap"
        >
          <Plus className="w-4 h-4" />
          {t('create')}
        </button>
      </div>

      {/* 搜索栏 + 内置过滤（黑神话式标签切换） */}
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-sage" />
          <input
            type="text"
            placeholder={t('searchPlaceholder')}
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className="dao-input pl-10"
          />
        </div>
        <TopTabs
          tabs={[
            { key: 'all', label: t('filter.all') },
            { key: 'builtin', label: t('filter.builtin') },
            { key: 'custom', label: t('filter.custom') },
          ]}
          activeKey={builtinFilter}
          onChange={key => setBuiltinFilter(key as BuiltinFilter)}
          className="sm:w-auto"
        />
      </div>

      {/* 创建金丹弹窗 */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between gap-2 mb-5">
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <FlaskConical className="w-5 h-5 text-primary shrink-0" />
                <h2 className="text-lg font-serif font-bold text-gold truncate">
                  {t('modal.title')}
                </h2>
              </div>
              <button
                aria-label={t('closeModal')}
                onClick={() => setShowCreate(false)}
                className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreate} className="space-y-4">
              <NuwaDistillPanel
                onApply={(draft) => {
                  setDistilledDraft(draft)
                  setName(draft.name)
                  setDescription(draft.description)
                }}
              />

              <div>
                <label className="dao-label">{t('modal.nameLabel')}</label>
                <input
                  type="text"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder={t('modal.namePlaceholder')}
                  className="dao-input"
                  autoFocus
                  required
                />
              </div>

              <div>
                <label className="dao-label">{t('modal.descLabel')}</label>
                <textarea
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder={t('modal.descPlaceholder')}
                  className="dao-textarea"
                  rows={3}
                />
                <p className="text-[10px] text-sage mt-1">
                  {t('modal.descHint')}
                </p>
              </div>

              <div className="flex items-center gap-3 pt-2 flex-wrap">
                <button
                  type="button"
                  onClick={() => setShowCreate(false)}
                  className="dao-btn-ghost flex-1 whitespace-nowrap"
                >
                  {t('modal.cancel')}
                </button>
                <button
                  type="submit"
                  disabled={!name.trim() || creating}
                  className="dao-btn-primary flex-1 disabled:opacity-50 whitespace-nowrap"
                >
                  {creating ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <FlaskConical className="w-4 h-4" />
                  )}
                  {t('modal.submit')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 加载状态 */}
      {state.loading && state.pills.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold animate-spin mb-3" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      )}

      {!state.loading && state.error && (
        <div className="dao-card flex flex-col items-center px-6 py-10 text-center">
          <AlertCircle className="mb-3 h-10 w-10 text-primary" />
          <h3 className="mb-1 font-medium text-foreground">{t('loadErrorTitle')}</h3>
          <p className="mb-4 max-w-xl break-words text-sm text-muted-foreground">{state.error}</p>
          <button
            type="button"
            onClick={() => loadPills(searchQuery, builtinFilter)}
            className="dao-btn-ghost"
          >
            <RefreshCw className="h-4 w-4" />
            {t('retry')}
          </button>
        </div>
      )}

      {/* 空状态 */}
      {!state.loading && !state.error && state.pills.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <CircleDot className="w-12 h-12 text-sage/50 mb-3" />
          <h3 className="text-base font-medium text-muted-foreground mb-1">
            {searchQuery || builtinFilter !== 'all' ? t('emptySearchTitle') : t('emptyTitle')}
          </h3>
          <p className="text-sm text-sage mb-4">
            {searchQuery || builtinFilter !== 'all' ? t('emptySearchDesc') : t('emptyDesc')}
          </p>
          {!searchQuery && builtinFilter === 'all' && (
            <button onClick={() => setShowCreate(true)} className="dao-btn-primary whitespace-nowrap">
              <Plus className="w-4 h-4" />
              {t('create')}
            </button>
          )}
        </div>
      )}

      {/* 金丹列表 - 桌面端网格，H5 单列 */}
      {state.pills.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {state.pills.map(pill => (
            <PillCard key={pill.id} pill={pill} onBind={setBindingPill} />
          ))}
        </div>
      )}

      {/* 从金丹到道人 - 快捷绑定弹窗 */}
      {bindingPill && (
        <BindAgentModal pill={bindingPill} onClose={() => setBindingPill(null)} />
      )}
    </div>
  )
}

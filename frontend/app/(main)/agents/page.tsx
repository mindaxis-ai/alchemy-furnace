'use client'

/**
 * 道人府页面 - AI Agent 管理
 * 道人卡片列表（道教仙人风格头像占位）
 * 创建道人按钮 + 表单
 * H5 优化: 单列卡片
 */
import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useTranslations } from 'next-intl'
import {
  Plus,
  Users,
  Loader2,
  X,
  UserPlus,
  Sparkles,
  Cpu,
  Search,
  AlertCircle,
  RefreshCw,
} from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import { AgentCard } from '@/components/agent-card'
import { NuwaDistillPanel } from '@/components/nuwa-distill-panel'
import * as modelService from '@/services/modelService'
import type { ModelOption } from '@/services/modelService'

export default function AgentsPage() {
  const t = useTranslations('agents')
  const { state, fetchAgents, addAgent } = useAgent()
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [personality, setPersonality] = useState('')
  const [modelOptions, setModelOptions] = useState<ModelOption[]>([])
  const [modelName, setModelName] = useState('')
  const [searchQuery, setSearchQuery] = useState('')

  // 初始化加载
  useEffect(() => {
    fetchAgents()
    // 加载已启用模型选项（默认模型排在首位并作为默认选择）
    modelService.options()
      .then(opts => {
        const sorted = [...opts].sort((a, b) => Number(b.is_default) - Number(a.is_default))
        setModelOptions(sorted)
        const def = sorted.find(o => o.is_default) || sorted[0]
        if (def) setModelName(prev => prev || def.name)
      })
      .catch(() => {
        // 模型选项加载失败时保留下拉为空态提示
      })
  }, [fetchAgents])

  /** 创建道人 */
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    const agent = await addAgent({
      name: name.trim(),
      model_name: modelName || undefined,
      personality: personality.trim() || undefined,
    })
    if (agent) {
      setShowCreate(false)
      setName('')
      setPersonality('')
      const def = modelOptions.find(o => o.is_default) || modelOptions[0]
      setModelName(def?.name || '')
    }
  }

  /** 过滤道人 */
  const filteredAgents = state.agents.filter(agent =>
    agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (agent.personality?.toLowerCase() || '').includes(searchQuery.toLowerCase())
  )

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-3">
            <Users className="w-6 h-6 text-gold" />
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

      {/* 搜索栏 */}
      <div className="relative mb-6">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <input
          type="text"
          placeholder={t('searchPlaceholder')}
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          className="dao-input pl-10"
        />
      </div>

      {/* 创建道人弹窗 */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-foreground/40 backdrop-blur-sm">
          <div className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between gap-2 mb-5">
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <UserPlus className="w-5 h-5 text-sage shrink-0" />
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
                  setName((current) => current || draft.name)
                  setPersonality(draft.persona_summary)
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
                <label className="dao-label">{t('modal.personaLabel')}</label>
                <textarea
                  value={personality}
                  onChange={e => setPersonality(e.target.value)}
                  placeholder={t('modal.personaPlaceholder')}
                  className="dao-textarea"
                  rows={4}
                />
                <p className="text-[10px] text-sage mt-1">
                  {t('modal.personaHint')}
                </p>
              </div>

              <div>
                <label className="dao-label flex items-center gap-1.5">
                  <Cpu className="w-3.5 h-3.5" />
                  {t('modal.modelLabel')}
                </label>
                {modelOptions.length === 0 ? (
                  <p className="text-xs text-muted-foreground bg-muted border border-border/70 rounded-lg px-3 py-2.5">
                    {t('modal.modelEmpty')}
                    <Link href="/settings" className="text-gold hover:text-gold/80 mx-1 whitespace-nowrap">
                      {t('modal.modelLink')}
                    </Link>
                    {t('modal.modelEmptySuffix')}
                  </p>
                ) : (
                  <select
                    value={modelName}
                    onChange={e => setModelName(e.target.value)}
                    className="dao-input"
                  >
                    {modelOptions.map(model => (
                      <option key={`${model.provider_name}/${model.name}`} value={model.name}>
                        {model.display_name || model.name}（{model.provider_display_name || model.provider_name}）{model.is_default ? ` · ${t('modal.defaultBadge')}` : ''}
                      </option>
                    ))}
                  </select>
                )}
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
                  disabled={!name.trim() || state.loading}
                  className="dao-btn-primary flex-1 disabled:opacity-50 whitespace-nowrap"
                >
                  {state.loading ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <Sparkles className="w-4 h-4" />
                  )}
                  {t('modal.submit')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 加载状态 */}
      {state.loading && state.agents.length === 0 && (
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
          <button type="button" onClick={fetchAgents} className="dao-btn-ghost">
            <RefreshCw className="h-4 w-4" />
            {t('retry')}
          </button>
        </div>
      )}

      {/* 空状态 */}
      {!state.loading && !state.error && filteredAgents.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Users className="w-12 h-12 text-muted-foreground/50 mb-3" />
          <h3 className="text-base font-medium text-muted-foreground mb-1">
            {searchQuery ? t('emptySearchTitle') : t('emptyTitle')}
          </h3>
          <p className="text-sm text-sage mb-4">
            {searchQuery ? t('emptySearchDesc') : t('emptyDesc')}
          </p>
          {!searchQuery && (
            <button onClick={() => setShowCreate(true)} className="dao-btn-primary whitespace-nowrap">
              <Plus className="w-4 h-4" />
              {t('create')}
            </button>
          )}
        </div>
      )}

      {/* 道人列表 */}
      {filteredAgents.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredAgents.map(agent => (
            <AgentCard
              key={agent.id}
              agent={agent}
            />
          ))}
        </div>
      )}
    </div>
  )
}

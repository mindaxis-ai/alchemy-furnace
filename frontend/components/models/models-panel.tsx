'use client'

/**
 * 模型管理面板 - 供应商与模型配置（003 供应商协议化模型集成）；/settings 的 models tab 内容
 * 供应商卡片列表：显示名/协议徽标/掩码 Key/模型数/启停开关/连接测试/编辑/删除（409 提示模型数）
 * 新增供应商弹窗：第一步模板选择（国内/国际/本地分组 + 自定义），第二步表单（模板预填，base_url 可改）
 * 展开供应商可管理其下模型：列表/新增（模板建议模型快捷选择）/编辑/删除（409 引用保护）/启停
 * API Key 仅用于写入，编辑时以掩码占位，留空表示不修改
 */
import { useState, useEffect, useCallback } from 'react'
import { useTranslations } from 'next-intl'
import {
  Cpu,
  Plus,
  Loader2,
  X,
  Pencil,
  Trash2,
  Zap,
  Check,
  AlertCircle,
  Star,
  Wand2,
  FlaskConical,
  ChevronDown,
  ChevronRight,
  Server,
  ArrowLeft,
} from 'lucide-react'
import * as modelService from '@/services/modelService'
import { ApiError } from '@/services/api'
import type {
  Provider,
  ProviderTemplate,
  LLMModel,
  CreateProviderRequest,
  CreateModelRequest,
} from '@/services/modelService'

/** 分组展示顺序 */
const GROUP_ORDER = ['domestic', 'international', 'local'] as const

/** 协议徽标颜色 */
const PROTOCOL_BADGE: Record<string, string> = {
  'openai-compatible': 'bg-blue-500/10 text-blue-600 border-blue-500/20',
}

const DEFAULT_PROTOCOL_BADGE = 'bg-muted text-muted-foreground border-border/70'

/** 供应商表单状态 */
interface ProviderForm {
  name: string
  display_name: string
  protocol: string
  base_url: string
  api_key: string
  is_enabled: boolean
  sort_order: number
  remark: string
}

const EMPTY_PROVIDER_FORM: ProviderForm = {
  name: '',
  display_name: '',
  protocol: 'openai-compatible',
  base_url: '',
  api_key: '',
  is_enabled: true,
  sort_order: 0,
  remark: '',
}

/** 模型表单状态 */
interface ModelForm {
  name: string
  display_name: string
  temperature: number
  max_tokens: number
  is_enabled: boolean
  is_default: boolean
  is_synthesis: boolean
  is_fusion: boolean
  sort_order: number
}

const EMPTY_MODEL_FORM: ModelForm = {
  name: '',
  display_name: '',
  temperature: 0.7,
  max_tokens: 4096,
  is_enabled: true,
  is_default: false,
  is_synthesis: false,
  is_fusion: false,
  sort_order: 0,
}

/** 连接测试状态 */
interface TestState {
  loading: boolean
  result: { success: boolean; latency_ms: number; error: string } | null
}

export function ModelsPanel() {
  const t = useTranslations('modelsPanel')
  const tGroup = useTranslations('modelsPanel.group')

  const [providers, setProviders] = useState<Provider[]>([])
  const [templates, setTemplates] = useState<ProviderTemplate[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 展开的供应商及其模型
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [modelsByProvider, setModelsByProvider] = useState<Record<string, LLMModel[]>>({})
  const [modelsLoading, setModelsLoading] = useState<Record<string, boolean>>({})

  // 供应商弹窗：create = 两步（模板选择 → 表单），edit = 仅表单
  const [providerModal, setProviderModal] = useState<{
    mode: 'create' | 'edit'
    step: 1 | 2
    editing: Provider | null
    template: ProviderTemplate | null
  } | null>(null)
  const [providerForm, setProviderForm] = useState<ProviderForm>(EMPTY_PROVIDER_FORM)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  // 模型弹窗
  const [modelModal, setModelModal] = useState<{
    provider: Provider
    editing: LLMModel | null
  } | null>(null)
  const [modelForm, setModelForm] = useState<ModelForm>(EMPTY_MODEL_FORM)

  // 删除确认
  const [deletingProvider, setDeletingProvider] = useState<Provider | null>(null)
  const [deletingModel, setDeletingModel] = useState<LLMModel | null>(null)
  const [deleteLoading, setDeleteLoading] = useState(false)

  // 每个供应商的连接测试状态
  const [tests, setTests] = useState<Record<string, TestState>>({})

  /** 加载供应商列表 */
  const fetchProviders = useCallback(async () => {
    setLoading(true)
    try {
      const data = await modelService.listProviders({ page_size: 100 })
      setProviders(data.list || [])
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('errorLoadProviders'))
    } finally {
      setLoading(false)
    }
  }, [t])

  /** 加载模板列表（失败不阻塞主流程） */
  const fetchTemplates = useCallback(async () => {
    try {
      const data = await modelService.listTemplates()
      setTemplates(data || [])
    } catch {
      // 模板加载失败时仅影响新增流程的预填
    }
  }, [])

  useEffect(() => {
    fetchProviders()
    fetchTemplates()
  }, [fetchProviders, fetchTemplates])

  /** 加载某供应商下的模型 */
  const fetchModels = useCallback(async (providerId: string) => {
    setModelsLoading(prev => ({ ...prev, [providerId]: true }))
    try {
      const data = await modelService.listModels(providerId)
      setModelsByProvider(prev => ({ ...prev, [providerId]: data || [] }))
    } catch (e) {
      setError(e instanceof Error ? e.message : t('errorLoadModels'))
    } finally {
      setModelsLoading(prev => ({ ...prev, [providerId]: false }))
    }
  }, [t])

  /** 展开/收起供应商的模型面板 */
  const toggleExpand = (provider: Provider) => {
    if (expandedId === provider.id) {
      setExpandedId(null)
    } else {
      setExpandedId(provider.id)
      fetchModels(provider.id)
    }
  }

  /** 查找供应商对应的模板（用于模型名快捷选择）：按 name 匹配，其次按 base_url */
  const findTemplate = useCallback(
    (provider: Provider): ProviderTemplate | null => {
      return (
        templates.find(tpl => tpl.id === provider.name) ||
        templates.find(tpl => tpl.default_base_url === provider.base_url) ||
        null
      )
    },
    [templates]
  )

  // ---------- 供应商弹窗 ----------

  /** 打开新增供应商弹窗（第一步：模板选择） */
  const openCreateProvider = () => {
    setProviderModal({ mode: 'create', step: 1, editing: null, template: null })
    setProviderForm(EMPTY_PROVIDER_FORM)
    setFormError(null)
  }

  /** 打开编辑供应商弹窗（api_key 留空 = 不修改，占位符展示掩码） */
  const openEditProvider = (provider: Provider) => {
    setProviderModal({ mode: 'edit', step: 2, editing: provider, template: null })
    setProviderForm({
      name: provider.name,
      display_name: provider.display_name,
      protocol: provider.protocol,
      base_url: provider.base_url,
      api_key: '',
      is_enabled: provider.is_enabled,
      sort_order: provider.sort_order,
      remark: provider.remark || '',
    })
    setFormError(null)
  }

  /** 选择模板（null = 自定义空白模板），进入第二步并预填表单 */
  const pickTemplate = (template: ProviderTemplate | null) => {
    setProviderModal(prev =>
      prev ? { ...prev, step: 2, template } : prev
    )
    if (template) {
      setProviderForm({
        ...EMPTY_PROVIDER_FORM,
        name: template.id,
        display_name: template.display_name,
        protocol: template.protocol,
        base_url: template.default_base_url,
      })
    } else {
      setProviderForm(EMPTY_PROVIDER_FORM)
    }
  }

  /** 提交供应商创建/编辑 */
  const handleProviderSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!providerModal) return
    if (!providerForm.name.trim() || !providerForm.base_url.trim()) return
    setSaving(true)
    setFormError(null)

    const payload: CreateProviderRequest = {
      name: providerForm.name.trim(),
      display_name: providerForm.display_name.trim() || undefined,
      protocol: providerForm.protocol.trim() || undefined,
      base_url: providerForm.base_url.trim(),
      is_enabled: providerForm.is_enabled,
      sort_order: providerForm.sort_order,
      remark: providerForm.remark.trim() || undefined,
    }
    // api_key 仅在填写时提交：创建时写入，编辑时留空表示不修改
    if (providerForm.api_key) {
      payload.api_key = providerForm.api_key
    }

    try {
      if (providerModal.mode === 'edit' && providerModal.editing) {
        await modelService.updateProvider(providerModal.editing.id, payload)
      } else {
        await modelService.createProvider(payload)
      }
      setProviderModal(null)
      await fetchProviders()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : t('errorSave'))
    } finally {
      setSaving(false)
    }
  }

  /** 切换供应商启停 */
  const toggleProviderEnabled = async (provider: Provider) => {
    try {
      await modelService.updateProvider(provider.id, { is_enabled: !provider.is_enabled })
      await fetchProviders()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errorToggleProvider'))
    }
  }

  /** 删除供应商（409：下仍有模型） */
  const handleDeleteProvider = async () => {
    if (!deletingProvider) return
    setDeleteLoading(true)
    try {
      await modelService.deleteProvider(deletingProvider.id)
      setDeletingProvider(null)
      setError(null)
      if (expandedId === deletingProvider.id) setExpandedId(null)
      await fetchProviders()
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const modelCount =
          (err.data?.data as { model_count?: number } | undefined)?.model_count ??
          deletingProvider.model_count
        setError(err.message || t('deleteProviderBlocked', { count: modelCount }))
      } else {
        setError(err instanceof Error ? err.message : t('errorDelete'))
      }
      setDeletingProvider(null)
    } finally {
      setDeleteLoading(false)
    }
  }

  /** 测试供应商连接 */
  const handleTest = async (provider: Provider) => {
    setTests(prev => ({ ...prev, [provider.id]: { loading: true, result: null } }))
    try {
      const result = await modelService.testProviderConnection(provider.id)
      setTests(prev => ({ ...prev, [provider.id]: { loading: false, result } }))
    } catch (err) {
      setTests(prev => ({
        ...prev,
        [provider.id]: {
          loading: false,
          result: {
            success: false,
            latency_ms: 0,
            error: err instanceof Error ? err.message : t('errorTestConnection'),
          },
        },
      }))
    }
  }

  // ---------- 模型管理 ----------

  /** 打开新增模型弹窗 */
  const openCreateModel = (provider: Provider) => {
    setModelModal({ provider, editing: null })
    setModelForm(EMPTY_MODEL_FORM)
    setFormError(null)
  }

  /** 打开编辑模型弹窗 */
  const openEditModel = (provider: Provider, model: LLMModel) => {
    setModelModal({ provider, editing: model })
    setModelForm({
      name: model.name,
      display_name: model.display_name,
      temperature: model.temperature,
      max_tokens: model.max_tokens,
      is_enabled: model.is_enabled,
      is_default: model.is_default,
      is_synthesis: model.is_synthesis,
      is_fusion: model.is_fusion,
      sort_order: model.sort_order,
    })
    setFormError(null)
  }

  /** 提交模型创建/编辑 */
  const handleModelSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!modelModal) return
    if (!modelForm.name.trim()) return
    setSaving(true)
    setFormError(null)

    const payload: CreateModelRequest = {
      name: modelForm.name.trim(),
      display_name: modelForm.display_name.trim() || undefined,
      temperature: modelForm.temperature,
      max_tokens: modelForm.max_tokens,
      is_enabled: modelForm.is_enabled,
      is_default: modelForm.is_default,
      is_synthesis: modelForm.is_synthesis,
      is_fusion: modelForm.is_fusion,
      sort_order: modelForm.sort_order,
    }

    try {
      if (modelModal.editing) {
        await modelService.updateModel(modelModal.editing.id, payload)
      } else {
        await modelService.createModel(modelModal.provider.id, payload)
      }
      setModelModal(null)
      await fetchModels(modelModal.provider.id)
      await fetchProviders()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : t('errorSave'))
    } finally {
      setSaving(false)
    }
  }

  /** 切换模型启停 */
  const toggleModelEnabled = async (provider: Provider, model: LLMModel) => {
    try {
      await modelService.updateModel(model.id, { is_enabled: !model.is_enabled })
      await fetchModels(provider.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errorToggleModel'))
    }
  }

  /** 删除模型（409：仍被道人引用） */
  const handleDeleteModel = async () => {
    if (!deletingModel || expandedId === null) return
    setDeleteLoading(true)
    try {
      await modelService.deleteModel(deletingModel.id)
      setDeletingModel(null)
      setError(null)
      await fetchModels(expandedId)
      await fetchProviders()
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const referencedBy =
          (err.data?.data as { referenced_by?: number } | undefined)?.referenced_by ??
          deletingModel.referenced_by
        setError(err.message || t('deleteModelBlocked', { count: referencedBy }))
      } else {
        setError(err instanceof Error ? err.message : t('errorDelete'))
      }
      setDeletingModel(null)
    } finally {
      setDeleteLoading(false)
    }
  }

  /** 按分组整理模板 */
  const groupedTemplates = GROUP_ORDER
    .map(group => ({
      group,
      label: tGroup(group as 'domestic' | 'international' | 'local'),
      items: templates.filter(tpl => tpl.group === group),
    }))
    .filter(g => g.items.length > 0)

  // 当前供应商表单是否展示 API Key 输入（创建时看模板 requires_api_key，编辑时总是展示）
  const showApiKeyInput =
    providerModal?.mode === 'edit' || (providerModal?.template?.requires_api_key ?? true)

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            <Cpu className="w-6 h-6 text-gold" />
            <h1 className="page-title truncate">{t('title')}</h1>
          </div>
          <p className="page-subtitle">{t('subtitle')}</p>
        </div>

        <button onClick={openCreateProvider} className="dao-btn-primary self-start whitespace-nowrap">
          <Plus className="w-4 h-4" />
          {t('createProvider')}
        </button>
      </div>

      {/* 加载状态 */}
      {loading && providers.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold animate-spin mb-3" />
          <p className="text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      )}

      {/* 空状态 */}
      {!loading && providers.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Server className="w-12 h-12 text-sage/60 mb-3" />
          <h3 className="text-base font-medium text-muted-foreground mb-1">{t('emptyTitle')}</h3>
          <p className="text-sm text-sage/70">{t('emptyDesc')}</p>
        </div>
      )}

      {/* 供应商卡片列表 */}
      {providers.length > 0 && (
        <div className="space-y-3">
          {providers.map(provider => {
            const test = tests[provider.id]
            const expanded = expandedId === provider.id
            const models = modelsByProvider[provider.id] || []
            return (
              <div key={provider.id} className="dao-card overflow-hidden">
                {/* 供应商行 */}
                <div className="p-4 flex flex-col lg:flex-row lg:items-center gap-3">
                  {/* 展开按钮 + 名称 */}
                  <div className="flex items-center gap-2 flex-1 min-w-0">
                    <button
                      onClick={() => toggleExpand(provider)}
                      className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-gold/80 transition-colors shrink-0"
                      title={expanded ? t('collapseModels') : t('expandModels')}
                    >
                      {expanded ? (
                        <ChevronDown className="w-4 h-4" />
                      ) : (
                        <ChevronRight className="w-4 h-4" />
                      )}
                    </button>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-sm font-medium text-foreground truncate">
                          {provider.display_name || provider.name}
                        </span>
                        <span className={`
                          text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap shrink-0
                          ${PROTOCOL_BADGE[provider.protocol] || DEFAULT_PROTOCOL_BADGE}
                        `}>
                          {provider.protocol}
                        </span>
                        {!provider.is_enabled && (
                          <span className="text-[10px] px-2 py-0.5 rounded-full border bg-muted text-muted-foreground border-border/70 whitespace-nowrap shrink-0">
                            {t('disabledBadge')}
                          </span>
                        )}
                      </div>
                      <p className="text-[11px] text-sage/70 truncate mt-0.5 font-mono">
                        {provider.base_url}
                      </p>
                    </div>
                  </div>

                  {/* 密钥 / 模型数 */}
                  <div className="flex items-center gap-4 text-xs text-muted-foreground shrink-0">
                    <span className="font-mono whitespace-nowrap">
                      {provider.has_api_key ? provider.api_key_masked || t('keyConfigured') : t('keyFree')}
                    </span>
                    <span className="whitespace-nowrap">{t('modelCount', { count: provider.model_count })}</span>
                  </div>

                  {/* 启停开关 */}
                  <button
                    onClick={() => toggleProviderEnabled(provider)}
                    className={`
                      relative inline-flex h-5 w-9 shrink-0 rounded-full transition-colors
                      ${provider.is_enabled ? 'bg-sage/60' : 'bg-muted'}
                    `}
                    title={provider.is_enabled ? t('clickToDisable') : t('clickToEnable')}
                  >
                    <span className={`
                      inline-block h-4 w-4 mt-0.5 rounded-full bg-foreground transition-transform
                      ${provider.is_enabled ? 'translate-x-[18px]' : 'translate-x-0.5'}
                    `} />
                  </button>

                  {/* 操作 */}
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      onClick={() => handleTest(provider)}
                      disabled={test?.loading}
                      className="p-1.5 rounded hover:bg-sage/15 text-muted-foreground hover:text-sage transition-colors disabled:opacity-40"
                      title={t('testConnection')}
                    >
                      {test?.loading ? (
                        <Loader2 className="w-4 h-4 animate-spin" />
                      ) : (
                        <Zap className="w-4 h-4" />
                      )}
                    </button>
                    <button
                      onClick={() => openEditProvider(provider)}
                      className="p-1.5 rounded hover:bg-gold/10 text-muted-foreground hover:text-gold/80 transition-colors"
                      title={t('edit')}
                    >
                      <Pencil className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => setDeletingProvider(provider)}
                      className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors"
                      title={t('delete')}
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>

                {/* 连接测试结果 */}
                {test?.result && (
                  <div className={`
                    px-4 pb-3 text-[11px] flex items-center gap-1 flex-wrap min-w-0
                    ${test.result.success ? 'text-sage' : 'text-primary'}
                  `}>
                    {test.result.success ? (
                      <>
                        <Check className="w-3 h-3 shrink-0" />
                        <span className="whitespace-nowrap">{t('connectSuccess', { ms: test.result.latency_ms })}</span>
                      </>
                    ) : (
                      <>
                        <AlertCircle className="w-3 h-3 shrink-0" />
                        <span className="break-words min-w-0">{test.result.error || t('connectFailed')}</span>
                      </>
                    )}
                  </div>
                )}

                {/* 展开的模型面板 */}
                {expanded && (
                  <div className="border-t border-border/70 bg-card p-4 animate-in fade-in duration-300">
                    <div className="flex items-center justify-between mb-3 gap-2 flex-wrap">
                      <h3 className="text-sm font-medium text-gold">{t('modelList')}</h3>
                      <button
                        onClick={() => openCreateModel(provider)}
                        className="dao-btn-gold text-xs px-3 py-1.5 whitespace-nowrap"
                      >
                        <Plus className="w-3.5 h-3.5" />
                        {t('addModel')}
                      </button>
                    </div>

                    {modelsLoading[provider.id] ? (
                      <div className="flex items-center justify-center py-6">
                        <Loader2 className="w-5 h-5 text-gold animate-spin" />
                      </div>
                    ) : models.length === 0 ? (
                      <p className="text-xs text-sage/70 text-center py-6">
                        {t('emptyModels')}
                      </p>
                    ) : (
                      <div className="overflow-x-auto">
                        <table className="w-full text-sm min-w-[760px]">
                          <thead>
                            <tr className="border-b border-border/70 text-left">
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground whitespace-nowrap">{t('th.name')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground whitespace-nowrap">{t('th.displayName')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground whitespace-nowrap">{t('th.temperature')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center whitespace-nowrap">{t('th.default')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center whitespace-nowrap">{t('th.synthesis')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center whitespace-nowrap">{t('th.fusion')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center whitespace-nowrap">{t('th.enabled')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center whitespace-nowrap">{t('th.refCount')}</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-right whitespace-nowrap">{t('th.actions')}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {models.map(model => (
                              <tr
                                key={model.id}
                                className="border-b border-border/50 last:border-0 hover:bg-gold/5 transition-colors"
                              >
                                <td className="px-3 py-2.5 text-foreground font-medium whitespace-nowrap">
                                  {model.name}
                                </td>
                                <td className="px-3 py-2.5 text-foreground whitespace-nowrap">
                                  {model.display_name || '-'}
                                </td>
                                <td className="px-3 py-2.5 text-foreground whitespace-nowrap">{model.temperature}</td>
                                <td className="px-3 py-2.5 text-center">
                                  {model.is_default && <Star className="w-4 h-4 text-gold inline" />}
                                </td>
                                <td className="px-3 py-2.5 text-center">
                                  {model.is_synthesis && <Wand2 className="w-4 h-4 text-sage inline" />}
                                </td>
                                <td className="px-3 py-2.5 text-center">
                                  {model.is_fusion && <FlaskConical className="w-4 h-4 text-primary inline" />}
                                </td>
                                <td className="px-3 py-2.5 text-center">
                                  <button
                                    onClick={() => toggleModelEnabled(provider, model)}
                                    className={`
                                      inline-block w-2 h-2 rounded-full
                                      ${model.is_enabled ? 'bg-sage' : 'bg-sage/40'}
                                    `}
                                    title={model.is_enabled ? t('clickToDisable') : t('clickToEnable')}
                                  />
                                </td>
                                <td className="px-3 py-2.5 text-center text-foreground">
                                  {model.referenced_by}
                                </td>
                                <td className="px-3 py-2.5">
                                  <div className="flex items-center justify-end gap-1">
                                    <button
                                      onClick={() => openEditModel(provider, model)}
                                      className="p-1.5 rounded hover:bg-gold/10 text-muted-foreground hover:text-gold/80 transition-colors"
                                      title={t('edit')}
                                    >
                                      <Pencil className="w-4 h-4" />
                                    </button>
                                    <button
                                      onClick={() => setDeletingModel(model)}
                                      className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors"
                                      title={t('delete')}
                                    >
                                      <Trash2 className="w-4 h-4" />
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* 新增/编辑供应商弹窗 */}
      {providerModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-foreground/40 backdrop-blur-sm">
          <div className="dao-card w-full max-w-lg p-6 animate-in fade-in duration-300 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between gap-2 mb-5">
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <Server className="w-5 h-5 text-sage shrink-0" />
                <h2 className="text-lg font-serif font-bold text-gold truncate">
                  {providerModal.mode === 'edit'
                    ? t('providerModal.edit')
                    : providerModal.step === 1
                      ? t('providerModal.selectTemplate')
                      : t('providerModal.configure')}
                </h2>
              </div>
              <button
                onClick={() => setProviderModal(null)}
                className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 第一步：模板选择（分组展示 + 自定义） */}
            {providerModal.mode === 'create' && providerModal.step === 1 && (
              <div className="space-y-4">
                {groupedTemplates.map(g => (
                  <div key={g.group}>
                    <p className="text-xs text-muted-foreground mb-2">{g.label}</p>
                    <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                      {g.items.map(tpl => (
                        <button
                          key={tpl.id}
                          onClick={() => pickTemplate(tpl)}
                          className="p-3 rounded-lg bg-muted border border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left min-w-0"
                        >
                          <p className="text-sm text-foreground truncate">{tpl.display_name}</p>
                          <p className="text-[10px] text-sage/70 truncate mt-0.5">{tpl.id}</p>
                        </button>
                      ))}
                    </div>
                  </div>
                ))}
                <div>
                  <p className="text-xs text-muted-foreground mb-2">{t('groupOther')}</p>
                  <button
                    onClick={() => pickTemplate(null)}
                    className="w-full p-3 rounded-lg bg-muted border border-dashed border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left min-w-0"
                  >
                    <p className="text-sm text-foreground">{t('custom')}</p>
                    <p className="text-[10px] text-sage/70 mt-0.5">{t('customDesc')}</p>
                  </button>
                </div>
              </div>
            )}

            {/* 第二步：供应商表单 */}
            {providerModal.step === 2 && (
              <form onSubmit={handleProviderSubmit} className="space-y-4">
                {providerModal.mode === 'create' && (
                  <button
                    type="button"
                    onClick={() => setProviderModal(prev => prev ? { ...prev, step: 1 } : prev)}
                    className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-gold/80 transition-colors whitespace-nowrap"
                  >
                    <ArrowLeft className="w-3.5 h-3.5" />
                    {t('providerModal.reselectTemplate')}
                  </button>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div className="min-w-0">
                    <label className="dao-label">{t('providerNameLabel')}</label>
                    <input
                      type="text"
                      value={providerForm.name}
                      onChange={e => setProviderForm({ ...providerForm, name: e.target.value })}
                      placeholder={t('providerNamePlaceholder')}
                      className="dao-input"
                      required
                    />
                  </div>
                  <div className="min-w-0">
                    <label className="dao-label">{t('displayNameLabel')}</label>
                    <input
                      type="text"
                      value={providerForm.display_name}
                      onChange={e => setProviderForm({ ...providerForm, display_name: e.target.value })}
                      placeholder={t('displayNamePlaceholder')}
                      className="dao-input"
                    />
                  </div>
                </div>

                <div>
                  <label className="dao-label">{t('protocolLabel')}</label>
                  <input
                    type="text"
                    value={providerForm.protocol}
                    onChange={e => setProviderForm({ ...providerForm, protocol: e.target.value })}
                    placeholder={t('protocolPlaceholder')}
                    className="dao-input"
                  />
                  <p className="text-[10px] text-sage/70 mt-1">{t('protocolHint')}</p>
                </div>

                <div>
                  <label className="dao-label">{t('baseUrlLabel')}</label>
                  <input
                    type="text"
                    value={providerForm.base_url}
                    onChange={e => setProviderForm({ ...providerForm, base_url: e.target.value })}
                    placeholder={t('baseUrlPlaceholder')}
                    className="dao-input"
                    required
                  />
                </div>

                {showApiKeyInput && (
                  <div>
                    <label className="dao-label">{t('apiKeyLabel')}</label>
                    <input
                      type="password"
                      value={providerForm.api_key}
                      onChange={e => setProviderForm({ ...providerForm, api_key: e.target.value })}
                      placeholder={
                        providerModal.mode === 'edit' && providerModal.editing
                          ? providerModal.editing.has_api_key
                            ? providerModal.editing.api_key_masked || t('apiKeyConfiguredPlaceholder')
                            : t('apiKeyNotConfiguredPlaceholder')
                          : t('apiKeyPlaceholder')
                      }
                      className="dao-input"
                      autoComplete="new-password"
                    />
                    {providerModal.mode === 'edit' && (
                      <p className="text-[10px] text-sage/70 mt-1">
                        {t('apiKeyEditHint')}
                      </p>
                    )}
                  </div>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div className="min-w-0">
                    <label className="dao-label">{t('sortOrderLabel')}</label>
                    <input
                      type="number"
                      min={0}
                      step={1}
                      value={providerForm.sort_order}
                      onChange={e => setProviderForm({ ...providerForm, sort_order: Math.max(0, Math.floor(Number(e.target.value))) })}
                      className="dao-input"
                    />
                  </div>
                  <div className="min-w-0">
                    <label className="dao-label">{t('remarkLabel')}</label>
                    <input
                      type="text"
                      value={providerForm.remark}
                      onChange={e => setProviderForm({ ...providerForm, remark: e.target.value })}
                      placeholder={t('optionalPlaceholder')}
                      className="dao-input"
                    />
                  </div>
                </div>

                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer whitespace-nowrap">
                  <input
                    type="checkbox"
                    checked={providerForm.is_enabled}
                    onChange={e => setProviderForm({ ...providerForm, is_enabled: e.target.checked })}
                    className="accent-sage w-4 h-4"
                  />
                  {t('enableLabel')}
                </label>

                {formError && (
                  <div className="flex items-center gap-2 text-sm text-primary bg-primary/10 border border-primary/20 rounded-lg px-3 py-2">
                    <AlertCircle className="w-4 h-4 shrink-0" />
                    <span className="break-words min-w-0">{formError}</span>
                  </div>
                )}

                <div className="flex items-center gap-3 pt-2 flex-wrap">
                  <button
                    type="button"
                    onClick={() => setProviderModal(null)}
                    className="dao-btn-ghost flex-1 whitespace-nowrap"
                  >
                    {t('cancel')}
                  </button>
                  <button
                    type="submit"
                    disabled={!providerForm.name.trim() || !providerForm.base_url.trim() || saving}
                    className="dao-btn-primary flex-1 disabled:opacity-50 whitespace-nowrap"
                  >
                    {saving ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <Check className="w-4 h-4" />
                    )}
                    {providerModal.mode === 'edit' ? t('saveCta') : t('createCta')}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* 新增/编辑模型弹窗 */}
      {modelModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-foreground/40 backdrop-blur-sm">
          <div className="dao-card w-full max-w-lg p-6 animate-in fade-in duration-300 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between gap-2 mb-5">
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <Cpu className="w-5 h-5 text-sage shrink-0" />
                <h2 className="text-lg font-serif font-bold text-gold truncate">
                  {modelModal.editing
                    ? t('modelModal.edit')
                    : t('modelModal.createForProvider', { name: modelModal.provider.display_name || modelModal.provider.name })}
                </h2>
              </div>
              <button
                onClick={() => setModelModal(null)}
                className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleModelSubmit} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="min-w-0">
                  <label className="dao-label">{t('modelNameLabel')}</label>
                  <input
                    type="text"
                    value={modelForm.name}
                    onChange={e => setModelForm({ ...modelForm, name: e.target.value })}
                    placeholder={t('modelNamePlaceholder')}
                    className="dao-input"
                    required
                  />
                  {/* 模板建议模型快捷选择 */}
                  {!modelModal.editing && (() => {
                    const suggested = findTemplate(modelModal.provider)?.suggested_models || []
                    if (suggested.length === 0) return null
                    return (
                      <div className="flex flex-wrap gap-1.5 mt-2">
                        {suggested.map(name => (
                          <button
                            key={name}
                            type="button"
                            onClick={() => setModelForm(prev => ({
                              ...prev,
                              name,
                              display_name: prev.display_name || name,
                            }))}
                            className={`
                              text-[10px] px-2 py-1 rounded-full border transition-colors whitespace-nowrap
                              ${modelForm.name === name
                                ? 'bg-gold/20 text-gold border-gold/40'
                                : 'bg-muted text-muted-foreground border-border/70 hover:border-gold/40 hover:text-gold/80'
                              }
                            `}
                          >
                            {name}
                          </button>
                        ))}
                      </div>
                    )
                  })()}
                </div>
                <div className="min-w-0">
                  <label className="dao-label">{t('displayNameLabel')}</label>
                  <input
                    type="text"
                    value={modelForm.display_name}
                    onChange={e => setModelForm({ ...modelForm, display_name: e.target.value })}
                    placeholder={t('displayNamePlaceholder')}
                    className="dao-input"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="min-w-0">
                  <label className="dao-label">{t('temperatureLabel')}</label>
                  <input
                    type="number"
                    min={0}
                    max={2}
                    step={0.1}
                    value={modelForm.temperature}
                    onChange={e => setModelForm({ ...modelForm, temperature: Number(e.target.value) })}
                    className="dao-input"
                  />
                </div>
                <div className="min-w-0">
                  <label className="dao-label">{t('maxTokensLabel')}</label>
                  <input
                    type="number"
                    min={1}
                    step={1}
                    value={modelForm.max_tokens}
                    onChange={e => setModelForm({ ...modelForm, max_tokens: Math.max(1, Math.floor(Number(e.target.value))) })}
                    className="dao-input"
                  />
                </div>
                <div className="min-w-0">
                  <label className="dao-label">{t('sortOrderLabel')}</label>
                  <input
                    type="number"
                    min={0}
                    step={1}
                    value={modelForm.sort_order}
                    onChange={e => setModelForm({ ...modelForm, sort_order: Math.max(0, Math.floor(Number(e.target.value))) })}
                    className="dao-input"
                  />
                </div>
              </div>

              <div className="flex flex-wrap gap-x-6 gap-y-2">
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer whitespace-nowrap">
                  <input
                    type="checkbox"
                    checked={modelForm.is_enabled}
                    onChange={e => setModelForm({ ...modelForm, is_enabled: e.target.checked })}
                    className="accent-sage w-4 h-4"
                  />
                  {t('enableLabel')}
                </label>
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer whitespace-nowrap">
                  <input
                    type="checkbox"
                    checked={modelForm.is_default}
                    onChange={e => setModelForm({ ...modelForm, is_default: e.target.checked })}
                    className="accent-gold w-4 h-4"
                  />
                  {t('setDefaultLabel')}
                </label>
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer whitespace-nowrap">
                  <input
                    type="checkbox"
                    checked={modelForm.is_synthesis}
                    onChange={e => setModelForm({ ...modelForm, is_synthesis: e.target.checked })}
                    className="accent-sage w-4 h-4"
                  />
                  {t('useForSynthesisLabel')}
                </label>
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer whitespace-nowrap">
                  <input
                    type="checkbox"
                    checked={modelForm.is_fusion}
                    onChange={e => setModelForm({ ...modelForm, is_fusion: e.target.checked })}
                    className="accent-primary w-4 h-4"
                  />
                  {t('useForFusionLabel')}
                </label>
              </div>

              {formError && (
                <div className="flex items-center gap-2 text-sm text-primary bg-primary/10 border border-primary/20 rounded-lg px-3 py-2">
                  <AlertCircle className="w-4 h-4 shrink-0" />
                  <span className="break-words min-w-0">{formError}</span>
                </div>
              )}

              <div className="flex items-center gap-3 pt-2 flex-wrap">
                <button
                  type="button"
                  onClick={() => setModelModal(null)}
                  className="dao-btn-ghost flex-1 whitespace-nowrap"
                >
                  {t('cancel')}
                </button>
                <button
                  type="submit"
                  disabled={!modelForm.name.trim() || saving}
                  className="dao-btn-primary flex-1 disabled:opacity-50 whitespace-nowrap"
                >
                  {saving ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <Check className="w-4 h-4" />
                  )}
                  {modelModal.editing ? t('saveCta') : t('createCta')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 删除供应商确认弹窗 */}
      {deletingProvider && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-foreground/40 backdrop-blur-sm">
          <div className="dao-card w-full max-w-sm p-6 animate-in fade-in duration-300">
            <div className="flex items-center gap-2 mb-4">
              <AlertCircle className="w-5 h-5 text-primary" />
              <h2 className="text-lg font-serif font-bold text-gold">{t('deleteProviderTitle')}</h2>
            </div>
            <p className="text-sm text-foreground mb-2">
              {t('deleteProviderDesc', { name: deletingProvider.display_name || deletingProvider.name })}
            </p>
            {deletingProvider.model_count > 0 && (
              <p className="text-xs text-gold/90 bg-gold/10 border border-gold/20 rounded-lg px-3 py-2 mb-2">
                {t('deleteProviderBlocked', { count: deletingProvider.model_count })}
              </p>
            )}
            <div className="flex items-center gap-3 mt-5 flex-wrap">
              <button
                onClick={() => setDeletingProvider(null)}
                className="dao-btn-ghost flex-1 whitespace-nowrap"
              >
                {t('cancel')}
              </button>
              <button
                onClick={handleDeleteProvider}
                disabled={deleteLoading}
                className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-primary/10 border border-primary/40 text-primary hover:bg-primary/20 transition-colors text-sm font-medium disabled:opacity-50 whitespace-nowrap"
              >
                {deleteLoading ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
                {t('deleteCta')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 删除模型确认弹窗 */}
      {deletingModel && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-foreground/40 backdrop-blur-sm">
          <div className="dao-card w-full max-w-sm p-6 animate-in fade-in duration-300">
            <div className="flex items-center gap-2 mb-4">
              <AlertCircle className="w-5 h-5 text-primary" />
              <h2 className="text-lg font-serif font-bold text-gold">{t('deleteModelTitle')}</h2>
            </div>
            <p className="text-sm text-foreground mb-2">
              {t('deleteModelDesc', { name: deletingModel.display_name || deletingModel.name })}
            </p>
            {deletingModel.referenced_by > 0 && (
              <p className="text-xs text-gold/90 bg-gold/10 border border-gold/20 rounded-lg px-3 py-2 mb-2">
                {t('deleteModelBlocked', { count: deletingModel.referenced_by })}
              </p>
            )}
            <div className="flex items-center gap-3 mt-5 flex-wrap">
              <button
                onClick={() => setDeletingModel(null)}
                className="dao-btn-ghost flex-1 whitespace-nowrap"
              >
                {t('cancel')}
              </button>
              <button
                onClick={handleDeleteModel}
                disabled={deleteLoading}
                className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-primary/10 border border-primary/40 text-primary hover:bg-primary/20 transition-colors text-sm font-medium disabled:opacity-50 whitespace-nowrap"
              >
                {deleteLoading ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
                {t('deleteCta')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 错误提示 */}
      {error && (
        <div className="fixed bottom-20 md:bottom-6 right-4 dao-card p-3 flex items-center gap-2 text-sm text-primary animate-in fade-in duration-300 z-50">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span className="break-words min-w-0">{error}</span>
          <button
            onClick={() => setError(null)}
            className="p-1 rounded hover:bg-muted text-muted-foreground shrink-0"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}
    </div>
  )
}

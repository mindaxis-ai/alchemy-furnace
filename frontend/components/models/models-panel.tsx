'use client'

/**
 * 模型管理面板 - 供应商与模型配置（003 供应商协议化模型集成）；/settings 的 models tab 内容
 * 供应商卡片列表：显示名/协议徽标/掩码 Key/模型数/启停开关/连接测试/编辑/删除（409 提示模型数）
 * 新增供应商弹窗：第一步模板选择（国内/国际/本地分组 + 自定义），第二步表单（模板预填，base_url 可改）
 * 展开供应商可管理其下模型：列表/新增（模板建议模型快捷选择）/编辑/删除（409 引用保护）/启停
 * API Key 仅用于写入，编辑时以掩码占位，留空表示不修改
 */
import { useState, useEffect, useCallback } from 'react'
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

/** 模板分组中文标签 */
const GROUP_LABELS: Record<string, string> = {
  domestic: '国内',
  international: '国际',
  local: '本地',
}

/** 分组展示顺序 */
const GROUP_ORDER = ['domestic', 'international', 'local']

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
  sort_order: 0,
}

/** 连接测试状态 */
interface TestState {
  loading: boolean
  result: { success: boolean; latency_ms: number; error: string } | null
}

export function ModelsPanel() {
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
      setError(e instanceof Error ? e.message : '获取供应商列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

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
      setError(e instanceof Error ? e.message : '获取模型列表失败')
    } finally {
      setModelsLoading(prev => ({ ...prev, [providerId]: false }))
    }
  }, [])

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
        templates.find(t => t.id === provider.name) ||
        templates.find(t => t.default_base_url === provider.base_url) ||
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
      setFormError(err instanceof Error ? err.message : '保存失败')
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
      setError(err instanceof Error ? err.message : '更新供应商状态失败')
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
        setError(err.message || `该供应商下仍有 ${modelCount} 个模型，无法删除`)
      } else {
        setError(err instanceof Error ? err.message : '删除失败')
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
            error: err instanceof Error ? err.message : '连接测试失败',
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
      setFormError(err instanceof Error ? err.message : '保存失败')
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
      setError(err instanceof Error ? err.message : '更新模型状态失败')
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
        setError(err.message || `该模型仍被 ${referencedBy} 个道人引用，无法删除`)
      } else {
        setError(err instanceof Error ? err.message : '删除失败')
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
      label: GROUP_LABELS[group] || group,
      items: templates.filter(t => t.group === group),
    }))
    .filter(g => g.items.length > 0)

  // 当前供应商表单是否展示 API Key 输入（创建时看模板 requires_api_key，编辑时总是展示）
  const showApiKeyInput =
    providerModal?.mode === 'edit' || (providerModal?.template?.requires_api_key ?? true)

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-3">
            <Cpu className="w-6 h-6 text-gold" />
            <h1 className="page-title">模型管理</h1>
          </div>
          <p className="page-subtitle">配置模型供应商及其下的语言模型，凭证一次配置全模型复用</p>
        </div>

        <button onClick={openCreateProvider} className="dao-btn-primary self-start">
          <Plus className="w-4 h-4" />
          新增供应商
        </button>
      </div>

      {/* 加载状态 */}
      {loading && providers.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold animate-spin mb-3" />
          <p className="text-sm text-muted-foreground">正在加载供应商...</p>
        </div>
      )}

      {/* 空状态 */}
      {!loading && providers.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Server className="w-12 h-12 text-sage/60 mb-3" />
          <h3 className="text-base font-medium text-muted-foreground mb-1">暂无供应商</h3>
          <p className="text-sm text-sage/70 mb-4">点击上方按钮，从预置模板添加第一个模型供应商</p>
          <button onClick={openCreateProvider} className="dao-btn-primary">
            <Plus className="w-4 h-4" />
            新增供应商
          </button>
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
                <div className="p-4 flex flex-col md:flex-row md:items-center gap-3">
                  {/* 展开按钮 + 名称 */}
                  <div className="flex items-center gap-2 flex-1 min-w-0">
                    <button
                      onClick={() => toggleExpand(provider)}
                      className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-gold/80 transition-colors flex-shrink-0"
                      title={expanded ? '收起模型列表' : '展开模型列表'}
                    >
                      {expanded ? (
                        <ChevronDown className="w-4 h-4" />
                      ) : (
                        <ChevronRight className="w-4 h-4" />
                      )}
                    </button>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-sm font-medium text-foreground">
                          {provider.display_name || provider.name}
                        </span>
                        <span className={`
                          text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap
                          ${PROTOCOL_BADGE[provider.protocol] || DEFAULT_PROTOCOL_BADGE}
                        `}>
                          {provider.protocol}
                        </span>
                        {!provider.is_enabled && (
                          <span className="text-[10px] px-2 py-0.5 rounded-full border bg-muted text-muted-foreground border-border/70">
                            已停用
                          </span>
                        )}
                      </div>
                      <p className="text-[11px] text-sage/70 truncate mt-0.5 font-mono">
                        {provider.base_url}
                      </p>
                    </div>
                  </div>

                  {/* 密钥 / 模型数 */}
                  <div className="flex items-center gap-4 text-xs text-muted-foreground flex-shrink-0">
                    <span className="font-mono">
                      {provider.has_api_key ? provider.api_key_masked || '已配置' : '免密钥'}
                    </span>
                    <span className="whitespace-nowrap">{provider.model_count} 个模型</span>
                  </div>

                  {/* 启停开关 */}
                  <button
                    onClick={() => toggleProviderEnabled(provider)}
                    className={`
                      relative inline-flex h-5 w-9 flex-shrink-0 rounded-full transition-colors
                      ${provider.is_enabled ? 'bg-sage/60' : 'bg-muted'}
                    `}
                    title={provider.is_enabled ? '点击停用' : '点击启用'}
                  >
                    <span className={`
                      inline-block h-4 w-4 mt-0.5 rounded-full bg-foreground transition-transform
                      ${provider.is_enabled ? 'translate-x-[18px]' : 'translate-x-0.5'}
                    `} />
                  </button>

                  {/* 操作 */}
                  <div className="flex items-center gap-1 flex-shrink-0">
                    <button
                      onClick={() => handleTest(provider)}
                      disabled={test?.loading}
                      className="p-1.5 rounded hover:bg-sage/15 text-muted-foreground hover:text-sage transition-colors disabled:opacity-40"
                      title="测试连接"
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
                      title="编辑"
                    >
                      <Pencil className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => setDeletingProvider(provider)}
                      className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors"
                      title="删除"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>

                {/* 连接测试结果 */}
                {test?.result && (
                  <div className={`
                    px-4 pb-3 text-[11px] flex items-center gap-1
                    ${test.result.success ? 'text-sage' : 'text-primary'}
                  `}>
                    {test.result.success ? (
                      <>
                        <Check className="w-3 h-3" />
                        连接成功 · {test.result.latency_ms}ms
                      </>
                    ) : (
                      <>
                        <AlertCircle className="w-3 h-3" />
                        {test.result.error || '连接失败'}
                      </>
                    )}
                  </div>
                )}

                {/* 展开的模型面板 */}
                {expanded && (
                  <div className="border-t border-border/70 bg-card p-4 animate-in fade-in duration-300">
                    <div className="flex items-center justify-between mb-3">
                      <h3 className="text-sm font-medium text-gold">模型列表</h3>
                      <button
                        onClick={() => openCreateModel(provider)}
                        className="dao-btn-gold text-xs px-3 py-1.5"
                      >
                        <Plus className="w-3.5 h-3.5" />
                        新增模型
                      </button>
                    </div>

                    {modelsLoading[provider.id] ? (
                      <div className="flex items-center justify-center py-6">
                        <Loader2 className="w-5 h-5 text-gold animate-spin" />
                      </div>
                    ) : models.length === 0 ? (
                      <p className="text-xs text-sage/70 text-center py-6">
                        该供应商下暂无模型，点击「新增模型」添加
                      </p>
                    ) : (
                      <div className="overflow-x-auto">
                        <table className="w-full text-sm min-w-[760px]">
                          <thead>
                            <tr className="border-b border-border/70 text-left">
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground">名称</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground">显示名</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground">温度</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center">默认</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center">合成</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center">启用</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-center">引用数</th>
                              <th className="px-3 py-2 text-xs font-medium text-muted-foreground text-right">操作</th>
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
                                <td className="px-3 py-2.5 text-foreground">{model.temperature}</td>
                                <td className="px-3 py-2.5 text-center">
                                  {model.is_default && <Star className="w-4 h-4 text-gold inline" />}
                                </td>
                                <td className="px-3 py-2.5 text-center">
                                  {model.is_synthesis && <Wand2 className="w-4 h-4 text-sage inline" />}
                                </td>
                                <td className="px-3 py-2.5 text-center">
                                  <button
                                    onClick={() => toggleModelEnabled(provider, model)}
                                    className={`
                                      inline-block w-2 h-2 rounded-full
                                      ${model.is_enabled ? 'bg-sage' : 'bg-sage/40'}
                                    `}
                                    title={model.is_enabled ? '点击停用' : '点击启用'}
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
                                      title="编辑"
                                    >
                                      <Pencil className="w-4 h-4" />
                                    </button>
                                    <button
                                      onClick={() => setDeletingModel(model)}
                                      className="p-1.5 rounded hover:bg-primary/10 text-muted-foreground hover:text-primary transition-colors"
                                      title="删除"
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
            <div className="flex items-center justify-between mb-5">
              <div className="flex items-center gap-2">
                <Server className="w-5 h-5 text-sage" />
                <h2 className="text-lg font-serif font-bold text-gold">
                  {providerModal.mode === 'edit'
                    ? '编辑供应商'
                    : providerModal.step === 1
                      ? '选择供应商模板'
                      : '配置供应商'}
                </h2>
              </div>
              <button
                onClick={() => setProviderModal(null)}
                className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
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
                      {g.items.map(t => (
                        <button
                          key={t.id}
                          onClick={() => pickTemplate(t)}
                          className="p-3 rounded-lg bg-muted border border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left"
                        >
                          <p className="text-sm text-foreground truncate">{t.display_name}</p>
                          <p className="text-[10px] text-sage/70 truncate mt-0.5">{t.id}</p>
                        </button>
                      ))}
                    </div>
                  </div>
                ))}
                <div>
                  <p className="text-xs text-muted-foreground mb-2">其他</p>
                  <button
                    onClick={() => pickTemplate(null)}
                    className="w-full p-3 rounded-lg bg-muted border border-dashed border-border/70 hover:border-gold/40 hover:bg-gold/5 transition-all text-left"
                  >
                    <p className="text-sm text-foreground">自定义</p>
                    <p className="text-[10px] text-sage/70 mt-0.5">手动填写供应商标识与接口地址</p>
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
                    className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-gold/80 transition-colors"
                  >
                    <ArrowLeft className="w-3.5 h-3.5" />
                    重新选择模板
                  </button>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="dao-label">供应商标识 *</label>
                    <input
                      type="text"
                      value={providerForm.name}
                      onChange={e => setProviderForm({ ...providerForm, name: e.target.value })}
                      placeholder="如：deepseek"
                      className="dao-input"
                      required
                    />
                  </div>
                  <div>
                    <label className="dao-label">显示名</label>
                    <input
                      type="text"
                      value={providerForm.display_name}
                      onChange={e => setProviderForm({ ...providerForm, display_name: e.target.value })}
                      placeholder="如：DeepSeek"
                      className="dao-input"
                    />
                  </div>
                </div>

                <div>
                  <label className="dao-label">协议类型</label>
                  <input
                    type="text"
                    value={providerForm.protocol}
                    onChange={e => setProviderForm({ ...providerForm, protocol: e.target.value })}
                    placeholder="openai-compatible"
                    className="dao-input"
                  />
                  <p className="text-[10px] text-sage/70 mt-1">当前仅支持 openai-compatible</p>
                </div>

                <div>
                  <label className="dao-label">Base URL *</label>
                  <input
                    type="text"
                    value={providerForm.base_url}
                    onChange={e => setProviderForm({ ...providerForm, base_url: e.target.value })}
                    placeholder="https://api.deepseek.com/v1"
                    className="dao-input"
                    required
                  />
                </div>

                {showApiKeyInput && (
                  <div>
                    <label className="dao-label">API Key</label>
                    <input
                      type="password"
                      value={providerForm.api_key}
                      onChange={e => setProviderForm({ ...providerForm, api_key: e.target.value })}
                      placeholder={
                        providerModal.mode === 'edit' && providerModal.editing
                          ? providerModal.editing.has_api_key
                            ? providerModal.editing.api_key_masked || '已配置密钥（留空不修改）'
                            : '未配置密钥'
                          : 'sk-xxxxxxxxxxxxxxxxxxxxxxxx'
                      }
                      className="dao-input"
                      autoComplete="new-password"
                    />
                    {providerModal.mode === 'edit' && (
                      <p className="text-[10px] text-sage/70 mt-1">
                        留空表示不修改密钥；填写新密钥将替换原密钥
                      </p>
                    )}
                  </div>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="dao-label">排序</label>
                    <input
                      type="number"
                      min={0}
                      step={1}
                      value={providerForm.sort_order}
                      onChange={e => setProviderForm({ ...providerForm, sort_order: Math.max(0, Math.floor(Number(e.target.value))) })}
                      className="dao-input"
                    />
                  </div>
                  <div>
                    <label className="dao-label">备注</label>
                    <input
                      type="text"
                      value={providerForm.remark}
                      onChange={e => setProviderForm({ ...providerForm, remark: e.target.value })}
                      placeholder="可选"
                      className="dao-input"
                    />
                  </div>
                </div>

                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                  <input
                    type="checkbox"
                    checked={providerForm.is_enabled}
                    onChange={e => setProviderForm({ ...providerForm, is_enabled: e.target.checked })}
                    className="accent-sage w-4 h-4"
                  />
                  启用
                </label>

                {formError && (
                  <div className="flex items-center gap-2 text-sm text-primary bg-primary/10 border border-primary/20 rounded-lg px-3 py-2">
                    <AlertCircle className="w-4 h-4 flex-shrink-0" />
                    <span>{formError}</span>
                  </div>
                )}

                <div className="flex items-center gap-3 pt-2">
                  <button
                    type="button"
                    onClick={() => setProviderModal(null)}
                    className="dao-btn-ghost flex-1"
                  >
                    取消
                  </button>
                  <button
                    type="submit"
                    disabled={!providerForm.name.trim() || !providerForm.base_url.trim() || saving}
                    className="dao-btn-primary flex-1 disabled:opacity-50"
                  >
                    {saving ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <Check className="w-4 h-4" />
                    )}
                    {providerModal.mode === 'edit' ? '保存' : '创建'}
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
            <div className="flex items-center justify-between mb-5">
              <div className="flex items-center gap-2">
                <Cpu className="w-5 h-5 text-sage" />
                <h2 className="text-lg font-serif font-bold text-gold">
                  {modelModal.editing ? '编辑模型' : `新增模型 · ${modelModal.provider.display_name || modelModal.provider.name}`}
                </h2>
              </div>
              <button
                onClick={() => setModelModal(null)}
                className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleModelSubmit} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="dao-label">模型名称 *</label>
                  <input
                    type="text"
                    value={modelForm.name}
                    onChange={e => setModelForm({ ...modelForm, name: e.target.value })}
                    placeholder="如：deepseek-chat"
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
                              text-[10px] px-2 py-1 rounded-full border transition-colors
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
                <div>
                  <label className="dao-label">显示名</label>
                  <input
                    type="text"
                    value={modelForm.display_name}
                    onChange={e => setModelForm({ ...modelForm, display_name: e.target.value })}
                    placeholder="如：DeepSeek-V3"
                    className="dao-input"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div>
                  <label className="dao-label">温度（0-2）</label>
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
                <div>
                  <label className="dao-label">最大 Token</label>
                  <input
                    type="number"
                    min={1}
                    step={1}
                    value={modelForm.max_tokens}
                    onChange={e => setModelForm({ ...modelForm, max_tokens: Math.max(1, Math.floor(Number(e.target.value))) })}
                    className="dao-input"
                  />
                </div>
                <div>
                  <label className="dao-label">排序</label>
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
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                  <input
                    type="checkbox"
                    checked={modelForm.is_enabled}
                    onChange={e => setModelForm({ ...modelForm, is_enabled: e.target.checked })}
                    className="accent-sage w-4 h-4"
                  />
                  启用
                </label>
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                  <input
                    type="checkbox"
                    checked={modelForm.is_default}
                    onChange={e => setModelForm({ ...modelForm, is_default: e.target.checked })}
                    className="accent-gold w-4 h-4"
                  />
                  设为默认模型
                </label>
                <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                  <input
                    type="checkbox"
                    checked={modelForm.is_synthesis}
                    onChange={e => setModelForm({ ...modelForm, is_synthesis: e.target.checked })}
                    className="accent-sage w-4 h-4"
                  />
                  用于丹性合成
                </label>
              </div>

              {formError && (
                <div className="flex items-center gap-2 text-sm text-primary bg-primary/10 border border-primary/20 rounded-lg px-3 py-2">
                  <AlertCircle className="w-4 h-4 flex-shrink-0" />
                  <span>{formError}</span>
                </div>
              )}

              <div className="flex items-center gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setModelModal(null)}
                  className="dao-btn-ghost flex-1"
                >
                  取消
                </button>
                <button
                  type="submit"
                  disabled={!modelForm.name.trim() || saving}
                  className="dao-btn-primary flex-1 disabled:opacity-50"
                >
                  {saving ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <Check className="w-4 h-4" />
                  )}
                  {modelModal.editing ? '保存' : '创建'}
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
              <h2 className="text-lg font-serif font-bold text-gold">删除供应商</h2>
            </div>
            <p className="text-sm text-foreground mb-2">
              确定要删除供应商「{deletingProvider.display_name || deletingProvider.name}」吗？此操作不可撤销。
            </p>
            {deletingProvider.model_count > 0 && (
              <p className="text-xs text-gold/90 bg-gold/10 border border-gold/20 rounded-lg px-3 py-2 mb-2">
                该供应商下仍有 {deletingProvider.model_count} 个模型，删除将被拒绝
              </p>
            )}
            <div className="flex items-center gap-3 mt-5">
              <button
                onClick={() => setDeletingProvider(null)}
                className="dao-btn-ghost flex-1"
              >
                取消
              </button>
              <button
                onClick={handleDeleteProvider}
                disabled={deleteLoading}
                className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-primary/10 border border-primary/40 text-primary hover:bg-primary/20 transition-colors text-sm font-medium disabled:opacity-50"
              >
                {deleteLoading ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
                删除
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
              <h2 className="text-lg font-serif font-bold text-gold">删除模型</h2>
            </div>
            <p className="text-sm text-foreground mb-2">
              确定要删除模型「{deletingModel.display_name || deletingModel.name}」吗？此操作不可撤销。
            </p>
            {deletingModel.referenced_by > 0 && (
              <p className="text-xs text-gold/90 bg-gold/10 border border-gold/20 rounded-lg px-3 py-2 mb-2">
                该模型正被 {deletingModel.referenced_by} 个道人引用，删除将被拒绝
              </p>
            )}
            <div className="flex items-center gap-3 mt-5">
              <button
                onClick={() => setDeletingModel(null)}
                className="dao-btn-ghost flex-1"
              >
                取消
              </button>
              <button
                onClick={handleDeleteModel}
                disabled={deleteLoading}
                className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-primary/10 border border-primary/40 text-primary hover:bg-primary/20 transition-colors text-sm font-medium disabled:opacity-50"
              >
                {deleteLoading ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
                删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 错误提示 */}
      {error && (
        <div className="fixed bottom-20 md:bottom-6 right-4 dao-card p-3 flex items-center gap-2 text-sm text-primary animate-in fade-in duration-300 z-50">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{error}</span>
          <button
            onClick={() => setError(null)}
            className="p-1 rounded hover:bg-muted text-muted-foreground"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}
    </div>
  )
}

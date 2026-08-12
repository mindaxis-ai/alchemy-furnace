/**
 * 模型服务 - 供应商与模型管理 API（003 供应商协议化模型集成）
 * 对接后端 /api/v1/providers 与 /api/v1/models（统一响应封装 { code, message, data }）
 * 凭证归属供应商：api_key 仅用于写入（创建/更新供应商），接口永不明文返回，
 * 前端不得记录或展示明文密钥
 */
import { get, post, put, del } from './api'
import type { PagedList } from './types'

/** 供应商模板分组 */
export type TemplateGroup = 'domestic' | 'international' | 'local' | string

/** 预置供应商模板（静态常量，由后端下发） */
export interface ProviderTemplate {
  /** 模板标识（如 deepseek / ollama） */
  id: string
  display_name: string
  /** 协议类型（当前仅 openai-compatible） */
  protocol: string
  /** 预填 Base URL（用户可改） */
  default_base_url: string
  /** 是否需要 API Key（Ollama 等本地服务为 false） */
  requires_api_key: boolean
  /** 建议模型名列表（新增模型时快捷选择） */
  suggested_models: string[]
  /** 分组：domestic 国内 / international 国际 / local 本地 */
  group: TemplateGroup
}

/** LLM 供应商（api_key 仅以掩码形式返回） */
export interface Provider {
  id: string
  /** 供应商标识（唯一，如 deepseek） */
  name: string
  /** 显示名（如 DeepSeek） */
  display_name: string
  /** 协议类型 */
  protocol: string
  base_url: string
  /** 掩码后的 API Key（如 sk-****abcd） */
  api_key_masked?: string
  /** 是否已配置 API Key */
  has_api_key: boolean
  is_enabled: boolean
  sort_order: number
  remark: string
  /** 该供应商下的模型数量 */
  model_count: number
  created_at: string
  updated_at: string
}

/** 创建供应商请求（api_key 为明文，仅用于写入） */
export interface CreateProviderRequest {
  name: string
  display_name?: string
  protocol?: string
  base_url: string
  api_key?: string
  is_enabled?: boolean
  sort_order?: number
  remark?: string
}

/**
 * 更新供应商请求（全字段可选）
 * api_key 三态语义：不传 = 不修改，传空字符串 = 清除，传值 = 重加密
 */
export type UpdateProviderRequest = Partial<CreateProviderRequest>

/** 供应商列表查询参数 */
export interface ProviderListParams {
  page?: number
  page_size?: number
}

/** LLM 模型（凭证在供应商上，模型不再持有 base_url/api_key） */
export interface LLMModel {
  id: string
  /** 所属供应商 ID */
  provider_id: string
  /** 模型标识（API 调用名，如 deepseek-chat） */
  name: string
  /** 显示名 */
  display_name: string
  temperature: number
  max_tokens: number
  is_enabled: boolean
  /** 是否为默认模型 */
  is_default: boolean
  /** 是否为语言模式合成模型 */
  is_synthesis: boolean
  /** 是否为金丹融合专用模型 */
  is_fusion: boolean
  sort_order: number
  /** 引用该模型的道人数量 */
  referenced_by: number
  created_at: string
  updated_at: string
}

/** 创建模型请求（挂载在供应商下） */
export interface CreateModelRequest {
  name: string
  display_name?: string
  temperature?: number
  max_tokens?: number
  is_enabled?: boolean
  is_default?: boolean
  is_synthesis?: boolean
  is_fusion?: boolean
  sort_order?: number
}

/** 更新模型请求（全字段可选） */
export type UpdateModelRequest = Partial<CreateModelRequest>

/** 道人表单下拉用的精简模型选项（003 新增供应商信息） */
export interface ModelOption {
  name: string
  display_name: string
  /** 所属供应商标识 */
  provider_name: string
  /** 所属供应商显示名 */
  provider_display_name: string
  is_default: boolean
}

/** 连接测试结果 */
export interface TestConnectionResult {
  success: boolean
  latency_ms: number
  error: string
}

/**
 * 获取预置供应商模板列表
 */
export function listTemplates(): Promise<ProviderTemplate[]> {
  return get<ProviderTemplate[]>('/providers/templates')
}

/**
 * 获取供应商列表
 */
export function listProviders(params: ProviderListParams = {}): Promise<PagedList<Provider>> {
  return get<PagedList<Provider>>('/providers', {
    page: params.page ?? 1,
    page_size: params.page_size ?? 50,
  })
}

/**
 * 创建供应商
 */
export function createProvider(data: CreateProviderRequest): Promise<Provider> {
  return post<Provider>('/providers', data)
}

/**
 * 更新供应商
 */
export function updateProvider(id: string, data: UpdateProviderRequest): Promise<Provider> {
  return put<Provider>(`/providers/${id}`, data)
}

/**
 * 删除供应商
 * 若供应商下仍有模型，后端返回 409（message 为中文描述，data.model_count 为模型数）
 */
export function deleteProvider(id: string): Promise<void> {
  return del<void>(`/providers/${id}`)
}

/**
 * 测试供应商连接（以供应商凭证发起一次最小 LLM 调用）
 * @param id 供应商 ID
 * @param model 可选；缺省用该供应商下第一个启用模型
 */
export function testProviderConnection(id: string, model?: string): Promise<TestConnectionResult> {
  return post<TestConnectionResult>(`/providers/${id}/test-connection`, model ? { model } : {})
}

/**
 * 获取供应商下的模型列表（含 referenced_by 引用数）
 */
export function listModels(providerId: string): Promise<LLMModel[]> {
  return get<LLMModel[]>(`/providers/${providerId}/models`)
}

/**
 * 在供应商下创建模型
 */
export function createModel(providerId: string, data: CreateModelRequest): Promise<LLMModel> {
  return post<LLMModel>(`/providers/${providerId}/models`, data)
}

/**
 * 更新模型
 */
export function updateModel(id: string, data: UpdateModelRequest): Promise<LLMModel> {
  return put<LLMModel>(`/models/${id}`, data)
}

/**
 * 删除模型
 * 若模型仍被道人引用，后端返回 409（message 为中文描述，data.referenced_by 为引用数）
 */
export function deleteModel(id: string): Promise<void> {
  return del<void>(`/models/${id}`)
}

/**
 * 获取已启用供应商下的启用模型精简列表（道人表单下拉用）
 */
export function options(): Promise<ModelOption[]> {
  return get<ModelOption[]>('/models/options')
}

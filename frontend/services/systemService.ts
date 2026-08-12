/**
 * 系统配置服务 - /api/v1/system/config
 * 用于获取全局配置(默认/合成/融合模型等);供 /fusion 页面 banner 展示当前融合模型配置
 */
import { get } from './api'

/** 实际配置的融合模型详情(供 /fusion banner) */
export interface FusionModelInfo {
  /** 是否已配置(is_fusion=true AND is_enabled=true) */
  configured: boolean
  /** API 调用模型名 */
  model_name: string
  /** 显示名 */
  model_display_name: string
  /** 供应商标识 */
  provider_name: string
  /** 供应商显示名 */
  provider_display_name: string
}

/** 系统配置响应 */
export interface SystemConfig {
  version: string
  /** 可用模型清单(env 配置,仅供参考) */
  models: string[]
  /** 默认对话模型(env 兜底名) */
  default_model: string
  /** 合成专用模型(env 兜底名) */
  synthesis_model: string
  /** 融合专用模型(env 兜底名,无 is_fusion 时使用) */
  fusion_model: string
  /** 实际配置的融合模型(/fusion banner 展示用) */
  fusion_model_info: FusionModelInfo | null
}

/** 获取系统配置 */
export function getSystemConfig(): Promise<SystemConfig> {
  return get<SystemConfig>('/system/config')
}

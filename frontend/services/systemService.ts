/**
 * 系统配置服务 - /api/v1/system/config
 * 用于获取全局配置(默认/合成/融合模型等);供 /fusion 页面 banner 展示当前融合模型配置
 */
import { get, post } from './api'

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

/** 版本信息(/api/v1/version 全模式) */
export interface VersionInfo {
  version: string
  commit: string
  buildDate: string
}

/** 获取版本信息(关于区) */
export function getVersion(): Promise<VersionInfo> {
  return get<VersionInfo>('/version')
}

/** 检查更新响应(/api/v1/update/check,仅 desktop) */
export interface UpdateCheckResult {
  has_update: boolean
  latest_version: string
  current_version: string
  notes: string
  page_url: string
  asset_name: string
  asset_size: number
}

/** 检查更新 */
export function checkUpdate(): Promise<UpdateCheckResult> {
  return get<UpdateCheckResult>('/update/check')
}

/** 触发更新(POST /api/v1/update/apply,仅 desktop) */
export function applyUpdate(): Promise<{ message: string }> {
  return post<{ message: string }>('/update/apply')
}

/** 查询更新进度(/api/v1/update/progress) */
export interface UpdateProgress {
  /** 0..100 下载中;110 待重启;负数错误 */
  progress: number
}

export function getUpdateProgress(): Promise<UpdateProgress> {
  return get<UpdateProgress>('/update/progress')
}

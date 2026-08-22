/**
 * 金丹服务 - 语言模式技能包管理 API
 * 对接后端 /api/v1/pills
 */
import { get, post, put, del } from './api'
import type { Pill, CreatePillRequest, UpdatePillRequest, PagedList, PillListParams, SkillSchema } from './types'

/**
 * 获取金丹列表（支持关键词搜索与内置过滤）
 */
export function listPills(params: PillListParams = {}): Promise<PagedList<Pill>> {
  return get<PagedList<Pill>>('/pills', {
    page: params.page ?? 1,
    page_size: params.page_size ?? 100,
    keyword: params.keyword,
    is_builtin: params.is_builtin,
  })
}

/**
 * 获取单个金丹详情
 */
export function getPill(id: string): Promise<Pill> {
  return get<Pill>(`/pills/${id}`)
}

/**
 * 创建金丹
 */
export function createPill(data: CreatePillRequest): Promise<Pill> {
  return post<Pill>('/pills', data)
}

/**
 * 更新金丹
 */
export function updatePill(id: string, data: UpdatePillRequest): Promise<Pill> {
  return put<Pill>(`/pills/${id}`, data)
}

/**
 * 删除金丹
 */
export function deletePill(id: string): Promise<void> {
  return del<void>(`/pills/${id}`)
}

/**
 * 制作副本：深复制指定金丹为自定义副本（新 UUID、is_builtin=false、名称追加「副本」）
 */
export function clonePill(id: string): Promise<Pill> {
  return post<Pill>(`/pills/${id}/clone`)
}

/**
 * 新建金丹时的默认空 skill_schema
 */
export function emptySkillSchema(): SkillSchema {
  return {
    identity_card: '',
    expression_dna: {
      sentence_length: 'mixed',
      formality: 0.5,
      vocabulary: [],
      taboo_words: [],
      rhythm: '',
      humor_type: '',
      certainty_style: '',
      citation_habit: '',
    },
    mental_models: [],
    decision_heuristics: [],
    values: [],
    anti_patterns: [],
    honest_limits: [],
    example_dialogues: [],
  }
}

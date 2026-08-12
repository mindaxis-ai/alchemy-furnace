/**
 * 金丹融合服务 - 合丹为新 API
 * 对接后端 /api/v1/fusion/fuse(预览不落库);保存走 pillService.createPill
 */
import { post } from './api'
import type { Pill, SkillSchema } from './types'

export interface FuseOperator {
  id: string
  name: string
}

export interface FuseResult {
  name: string
  description: string
  skill_schema: SkillSchema
  operator: FuseOperator
  model: string
  degraded: boolean
}

/** 融合预览: N 枚金丹(≥2) -> 新金丹(不落库) */
export function fusePills(pillUuids: string[], excludeOperatorId?: string): Promise<FuseResult> {
  return post<FuseResult>('/fusion/fuse', {
    pill_uuids: pillUuids,
    ...(excludeOperatorId ? { exclude_operator_id: excludeOperatorId } : {}),
  })
}

/** 保存时注入血统到 skill_schema.fusion_lineage */
export function withLineage(
  skillSchema: SkillSchema,
  parents: Pill[],
  operator: FuseOperator,
): SkillSchema {
  return {
    ...skillSchema,
    fusion_lineage: {
      parents: parents.map(p => ({ uuid: p.id, name: p.name })),
      operator,
      fused_at: new Date().toISOString(),
    },
  }
}

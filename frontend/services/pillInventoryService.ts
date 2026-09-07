/**
 * 金丹库存与能力服务（金丹消耗品重构任务 6）
 * 库存实例单向流转 available → consumed_by_agent / consumed_by_fusion / discarded；
 * 服用消耗库存但能力保留（快照）；融合两阶段：预览不消耗 → 确认原子消耗全部材料。
 * 写操作（弃置/服用/移除/融合确认）必须携带 Idempotency-Key（UUID），
 * 断线恢复先 getOperation() 再决定是否用原 key 重试。
 */
import { request } from './api'
import type {
  AgentEffect,
  AgentEffectsResponse,
  FusionPreview,
  PillItemDetail,
  PillItemListItem,
  PillItemListParams,
  PillLegacyPointer,
  PillOperationResult,
  UpdateEffectsResponse,
} from './types'

/** Idempotency-Key 请求头（每个明确用户动作一个 key；重试沿用原 key） */
function idempotencyHeaders(key: string): Record<string, string> {
  return { 'Idempotency-Key': key }
}

// ---------- 金丹库存实例 ----------

/** 可用库存分页；recipeId 非空时按丹方过滤 */
export function listPillItems(params: PillItemListParams = {}): Promise<{ total: number; items: PillItemListItem[] }> {
  return request('/pill-items', {
    method: 'GET',
    params: { page: params.page, size: params.size, recipe_id: params.recipe_id },
  })
}

/** 实例详情（任意状态；已消耗/弃置展示状态去向） */
export function getPillItem(itemId: string): Promise<PillItemDetail> {
  return request(`/pill-items/${encodeURIComponent(itemId)}`, { method: 'GET' })
}

/** 弃置金丹：available→discarded 终态（显式确认，不物理删除；幂等） */
export function discardPillItem(key: string, itemId: string): Promise<PillOperationResult> {
  return request(`/pill-items/${encodeURIComponent(itemId)}/discard`, {
    method: 'POST',
    headers: idempotencyHeaders(key),
    body: JSON.stringify({}),
  })
}

/**
 * 旧金丹 ID 显式 legacy 解析（任务 5 封堵入口）：
 * 实例查询 404 后调用；命中返回 {entity_type:'recipe', recipe_id}，指向丹方详情。
 * 无映射时同样抛 404，由调用方决定展示「不存在」。
 */
export function resolveLegacyPill(pillId: string): Promise<PillLegacyPointer> {
  return request(`/pills/${encodeURIComponent(pillId)}`, { method: 'GET' })
}

// ---------- 服用与能力编排 ----------

/**
 * 服用金丹：available→consumed_by_agent + 能力快照（幂等）。
 * 成功后能力独立于原金丹存在；再次使用需按丹方炼制新实例。
 */
export function consumePill(
  key: string,
  agentId: string,
  itemId: string,
  opts: { weight?: number; sortOrder?: number } = {}
): Promise<PillOperationResult> {
  return request(`/agents/${encodeURIComponent(agentId)}/consume`, {
    method: 'POST',
    headers: idempotencyHeaders(key),
    body: JSON.stringify({
      item_id: itemId,
      ...(opts.weight !== undefined ? { weight: opts.weight } : {}),
      ...(opts.sortOrder !== undefined ? { sort_order: opts.sortOrder } : {}),
    }),
  })
}

/** 道人活跃能力列表（按 sort_order 升序；仅操作 effectId，与库存实例解耦） */
export function listEffects(agentId: string): Promise<AgentEffectsResponse> {
  return request(`/agents/${encodeURIComponent(agentId)}/effects`, { method: 'GET' })
}

/**
 * 能力全量编排（调权重/排序/保存道人信息；提交集必须等于活跃集）。
 * expectedEffectsRevision 乐观锁过期 → 409，调用方需重读后重试。
 */
export function updateEffects(
  agentId: string,
  expectedEffectsRevision: number,
  effects: Array<{ effectId: string; weight: number; sortOrder: number }>
): Promise<UpdateEffectsResponse> {
  return request(`/agents/${encodeURIComponent(agentId)}/effects`, {
    method: 'PUT',
    body: JSON.stringify({
      expected_effects_revision: expectedEffectsRevision,
      effects: effects.map((e) => ({
        effect_id: e.effectId,
        weight: e.weight,
        sort_order: e.sortOrder,
      })),
    }),
  })
}

/** 显式移除能力（按能力 UUID；软删保留历史，原实例不返还；幂等） */
export function removeEffect(key: string, agentId: string, effectId: string): Promise<PillOperationResult> {
  return request(`/agents/${encodeURIComponent(agentId)}/effects/${encodeURIComponent(effectId)}/remove`, {
    method: 'POST',
    headers: idempotencyHeaders(key),
    body: JSON.stringify({}),
  })
}

// ---------- 融合（两阶段） ----------

/** 融合预览：校验材料 → 模型生成 → 持久化预览（15 分钟 TTL；不消耗材料） */
export function previewFusion(
  itemIds: string[],
  excludeOperatorId?: string
): Promise<FusionPreview> {
  return request('/fusion/previews', {
    method: 'POST',
    body: JSON.stringify({
      item_ids: itemIds,
      ...(excludeOperatorId ? { exclude_operator_id: excludeOperatorId } : {}),
    }),
  })
}

/**
 * 原子确认融合：扣全部材料（available→consumed_by_fusion）并产出新金丹（幂等）。
 * 同一 preview 只能确认一次；重复请求用原 key 返回同一结果。
 */
export function confirmFusion(
  key: string,
  previewId: string,
  name: string,
  description = ''
): Promise<PillOperationResult> {
  return request('/fusion/confirm', {
    method: 'POST',
    headers: idempotencyHeaders(key),
    body: JSON.stringify({ preview_id: previewId, name, description }),
  })
}

// ---------- 幂等操作查询 ----------

/**
 * 读取已提交操作结果（幂等键 = 操作 ID；断线恢复先查此接口）。
 * 404 仅说明没有已提交结果，仍可用同 key 重试，不能自动换 key。
 */
export function getOperation(operationId: string): Promise<PillOperationResult> {
  return request(`/pill-operations/${encodeURIComponent(operationId)}`, { method: 'GET' })
}

export type { AgentEffect }

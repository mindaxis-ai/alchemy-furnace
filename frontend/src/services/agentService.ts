/**
 * 道人服务 - AI Agent 管理 API
 * 对接后端 /api/v1/agents，含服用金丹（绑定/权重/顺序）操作
 */
import { get, post, put, del } from './api'
import type {
  Agent,
  AgentDetail,
  CreateAgentRequest,
  UpdateAgentRequest,
  PagedList,
  AgentListParams,
} from './types'

/**
 * 获取道人列表
 */
export function listAgents(params: AgentListParams = {}): Promise<PagedList<Agent>> {
  return get<PagedList<Agent>>('/agents', {
    page: params.page ?? 1,
    page_size: params.page_size ?? 100,
    status: params.status,
  })
}

/**
 * 获取道人详情（含已服用金丹 agent_pills 与语言模式缓存）
 */
export function getAgent(id: number): Promise<AgentDetail> {
  return get<AgentDetail>(`/agents/${id}`)
}

/**
 * 创建道人
 */
export function createAgent(data: CreateAgentRequest): Promise<Agent> {
  return post<Agent>('/agents', data)
}

/**
 * 更新道人
 */
export function updateAgent(id: number, data: UpdateAgentRequest): Promise<Agent> {
  return put<Agent>(`/agents/${id}`, data)
}

/**
 * 删除道人
 */
export function deleteAgent(id: number): Promise<void> {
  return del<void>(`/agents/${id}`)
}

/**
 * 道人服用金丹（绑定），weight 0-10，sort_order >= 0
 */
export function bindPill(agentId: number, pillId: number, weight = 1, sortOrder = 0): Promise<void> {
  return post<void>(`/agents/${agentId}/pills`, {
    pill_id: pillId,
    weight,
    sort_order: sortOrder,
  })
}

/**
 * 更新服用记录（权重/顺序）
 */
export function updateAgentPill(
  agentId: number,
  pillId: number,
  weight: number,
  sortOrder: number
): Promise<void> {
  return put<void>(`/agents/${agentId}/pills/${pillId}`, {
    weight,
    sort_order: sortOrder,
  })
}

/**
 * 道人解除金丹绑定
 */
export function unbindPill(agentId: number, pillId: number): Promise<void> {
  return del<void>(`/agents/${agentId}/pills/${pillId}`)
}

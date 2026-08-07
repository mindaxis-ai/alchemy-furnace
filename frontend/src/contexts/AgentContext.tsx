/**
 * 道人状态管理 Context
 * 使用 React Context + useReducer 管理道人（AI Agent）相关状态
 * 道人详情包含已服用金丹（weight/sort_order）与语言模式缓存
 */
import React, { createContext, useContext, useReducer, useCallback } from 'react'
import * as agentService from '@/services/agentService'
import type { Agent, AgentDetail, CreateAgentRequest, UpdateAgentRequest } from '@/services/types'

/** 道人状态 */
interface AgentState {
  agents: Agent[]
  total: number
  currentAgent: AgentDetail | null
  loading: boolean
  error: string | null
}

/** 操作类型 */
type AgentAction =
  | { type: 'SET_AGENTS'; payload: { list: Agent[]; total: number } }
  | { type: 'SET_CURRENT_AGENT'; payload: AgentDetail | null }
  | { type: 'ADD_AGENT'; payload: Agent }
  | { type: 'UPDATE_AGENT'; payload: Agent }
  | { type: 'REMOVE_AGENT'; payload: number }
  | { type: 'SET_LOADING'; payload: boolean }
  | { type: 'SET_ERROR'; payload: string | null }

/** 初始状态 */
const initialState: AgentState = {
  agents: [],
  total: 0,
  currentAgent: null,
  loading: false,
  error: null,
}

/** Reducer */
function agentReducer(state: AgentState, action: AgentAction): AgentState {
  switch (action.type) {
    case 'SET_AGENTS':
      return { ...state, agents: action.payload.list, total: action.payload.total, loading: false }
    case 'SET_CURRENT_AGENT':
      return { ...state, currentAgent: action.payload, loading: false }
    case 'ADD_AGENT':
      return { ...state, agents: [action.payload, ...state.agents], loading: false }
    case 'UPDATE_AGENT':
      return {
        ...state,
        agents: state.agents.map(a => (a.id === action.payload.id ? { ...a, ...action.payload } : a)),
        currentAgent: state.currentAgent?.id === action.payload.id
          ? { ...state.currentAgent, ...action.payload }
          : state.currentAgent,
        loading: false,
      }
    case 'REMOVE_AGENT':
      return {
        ...state,
        agents: state.agents.filter(a => a.id !== action.payload),
        currentAgent: state.currentAgent?.id === action.payload ? null : state.currentAgent,
      }
    case 'SET_LOADING':
      return { ...state, loading: action.payload }
    case 'SET_ERROR':
      return { ...state, error: action.payload, loading: false }
    default:
      return state
  }
}

/** Context 类型 */
interface AgentContextType {
  state: AgentState
  dispatch: React.Dispatch<AgentAction>
  // 异步操作
  fetchAgents: () => Promise<void>
  fetchAgent: (id: number) => Promise<void>
  addAgent: (data: CreateAgentRequest) => Promise<Agent | null>
  editAgent: (id: number, data: UpdateAgentRequest) => Promise<Agent | null>
  removeAgent: (id: number) => Promise<boolean>
  /** 服用金丹（绑定），成功后刷新道人详情 */
  bindPill: (agentId: number, pillId: number, weight?: number, sortOrder?: number) => Promise<boolean>
  /** 更新服用记录（权重/顺序），成功后刷新道人详情 */
  updateAgentPill: (agentId: number, pillId: number, weight: number, sortOrder: number) => Promise<boolean>
  /** 解除金丹绑定，成功后刷新道人详情 */
  unbindPill: (agentId: number, pillId: number) => Promise<boolean>
}

const AgentContext = createContext<AgentContextType | null>(null)

/** Provider 组件 */
export function AgentProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(agentReducer, initialState)

  /** 获取道人列表 */
  const fetchAgents = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const data = await agentService.listAgents()
      dispatch({ type: 'SET_AGENTS', payload: { list: data.list || [], total: data.total } })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '获取道人列表失败' })
    }
  }, [])

  /** 获取单个道人详情 */
  const fetchAgent = useCallback(async (id: number) => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const agent = await agentService.getAgent(id)
      dispatch({ type: 'SET_CURRENT_AGENT', payload: agent })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '获取道人详情失败' })
    }
  }, [])

  /** 创建道人 */
  const addAgent = useCallback(async (data: CreateAgentRequest): Promise<Agent | null> => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const agent = await agentService.createAgent(data)
      dispatch({ type: 'ADD_AGENT', payload: agent })
      return agent
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '创建道人失败' })
      return null
    }
  }, [])

  /** 更新道人 */
  const editAgent = useCallback(async (id: number, data: UpdateAgentRequest): Promise<Agent | null> => {
    try {
      const agent = await agentService.updateAgent(id, data)
      dispatch({ type: 'UPDATE_AGENT', payload: agent })
      return agent
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '更新道人失败' })
      return null
    }
  }, [])

  /** 删除道人 */
  const removeAgent = useCallback(async (id: number): Promise<boolean> => {
    try {
      await agentService.deleteAgent(id)
      dispatch({ type: 'REMOVE_AGENT', payload: id })
      return true
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '删除道人失败' })
      return false
    }
  }, [])

  /** 刷新当前道人详情的辅助逻辑 */
  const refreshAgent = useCallback(async (agentId: number) => {
    const agent = await agentService.getAgent(agentId)
    dispatch({ type: 'SET_CURRENT_AGENT', payload: agent })
  }, [])

  /** 服用金丹（绑定） */
  const bindPill = useCallback(async (agentId: number, pillId: number, weight = 1, sortOrder = 0): Promise<boolean> => {
    try {
      await agentService.bindPill(agentId, pillId, weight, sortOrder)
      await refreshAgent(agentId)
      return true
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '服用金丹失败' })
      return false
    }
  }, [refreshAgent])

  /** 更新服用记录（权重/顺序） */
  const updateAgentPill = useCallback(async (
    agentId: number,
    pillId: number,
    weight: number,
    sortOrder: number
  ): Promise<boolean> => {
    try {
      await agentService.updateAgentPill(agentId, pillId, weight, sortOrder)
      await refreshAgent(agentId)
      return true
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '更新服用记录失败' })
      return false
    }
  }, [refreshAgent])

  /** 解除金丹绑定 */
  const unbindPill = useCallback(async (agentId: number, pillId: number): Promise<boolean> => {
    try {
      await agentService.unbindPill(agentId, pillId)
      await refreshAgent(agentId)
      return true
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '解除绑定失败' })
      return false
    }
  }, [refreshAgent])

  return (
    <AgentContext.Provider
      value={{
        state,
        dispatch,
        fetchAgents,
        fetchAgent,
        addAgent,
        editAgent,
        removeAgent,
        bindPill,
        updateAgentPill,
        unbindPill,
      }}
    >
      {children}
    </AgentContext.Provider>
  )
}

/** Hook */
export function useAgent(): AgentContextType {
  const context = useContext(AgentContext)
  if (!context) {
    throw new Error('useAgent must be used within a AgentProvider')
  }
  return context
}

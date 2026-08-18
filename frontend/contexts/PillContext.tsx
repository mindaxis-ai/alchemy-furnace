'use client'

/**
 * 金丹状态管理 Context
 * 使用 React Context + useReducer 管理金丹（语言模式技能包）相关状态
 */
import React, { createContext, useContext, useReducer, useCallback, useRef } from 'react'
import * as pillService from '@/services/pillService'
import type { Pill, CreatePillRequest, UpdatePillRequest, PillListParams } from '@/services/types'

/** 金丹状态 */
interface PillState {
  pills: Pill[]
  total: number
  currentPill: Pill | null
  loading: boolean
  error: string | null
}

/** 操作类型 */
type PillAction =
  | { type: 'SET_PILLS'; payload: { list: Pill[]; total: number } }
  | { type: 'SET_CURRENT_PILL'; payload: Pill | null }
  | { type: 'ADD_PILL'; payload: Pill }
  | { type: 'UPDATE_PILL'; payload: Pill }
  | { type: 'REMOVE_PILL'; payload: string }
  | { type: 'SET_LOADING'; payload: boolean }
  | { type: 'SET_ERROR'; payload: string | null }

/** 初始状态 */
const initialState: PillState = {
  pills: [],
  total: 0,
  currentPill: null,
  loading: false,
  error: null,
}

/** Reducer */
function pillReducer(state: PillState, action: PillAction): PillState {
  switch (action.type) {
    case 'SET_PILLS':
      return { ...state, pills: action.payload.list, total: action.payload.total, loading: false }
    case 'SET_CURRENT_PILL':
      return { ...state, currentPill: action.payload, loading: false }
    case 'ADD_PILL':
      return { ...state, pills: [action.payload, ...state.pills], total: state.total + 1 }
    case 'UPDATE_PILL':
      return {
        ...state,
        pills: state.pills.map(p => (p.id === action.payload.id ? action.payload : p)),
        currentPill: state.currentPill?.id === action.payload.id ? action.payload : state.currentPill,
      }
    case 'REMOVE_PILL':
      return {
        ...state,
        pills: state.pills.filter(p => p.id !== action.payload),
        currentPill: state.currentPill?.id === action.payload ? null : state.currentPill,
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
interface PillContextType {
  state: PillState
  dispatch: React.Dispatch<PillAction>
  // 异步操作
  fetchPills: (params?: PillListParams) => Promise<void>
  fetchPill: (id: string) => Promise<void>
  /** 创建金丹，返回创建的金丹（失败返回 null） */
  addPill: (data: CreatePillRequest) => Promise<Pill | null>
  /** 更新金丹，返回更新后的金丹（失败返回 null） */
  editPill: (id: string, data: UpdatePillRequest) => Promise<Pill | null>
  removePill: (id: string) => Promise<boolean>
}

const PillContext = createContext<PillContextType | null>(null)

/** Provider 组件 */
export function PillProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(pillReducer, initialState)
  const listRequestRef = useRef(0)

  /** 获取金丹列表 */
  const fetchPills = useCallback(async (params: PillListParams = {}) => {
    const requestId = ++listRequestRef.current
    dispatch({ type: 'SET_ERROR', payload: null })
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const data = await pillService.listPills(params)
      if (requestId !== listRequestRef.current) return
      dispatch({ type: 'SET_PILLS', payload: { list: data.list || [], total: data.total } })
    } catch (error) {
      if (requestId !== listRequestRef.current) return
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '获取金丹列表失败' })
    }
  }, [])

  /** 获取单个金丹 */
  const fetchPill = useCallback(async (id: string) => {
    dispatch({ type: 'SET_ERROR', payload: null })
    dispatch({ type: 'SET_CURRENT_PILL', payload: null })
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const pill = await pillService.getPill(id)
      dispatch({ type: 'SET_CURRENT_PILL', payload: pill })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '获取金丹详情失败' })
    }
  }, [])

  /** 创建金丹 */
  const addPill = useCallback(async (data: CreatePillRequest): Promise<Pill | null> => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const pill = await pillService.createPill(data)
      dispatch({ type: 'ADD_PILL', payload: pill })
      dispatch({ type: 'SET_LOADING', payload: false })
      return pill
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '创建金丹失败' })
      return null
    }
  }, [])

  /** 更新金丹 */
  const editPill = useCallback(async (id: string, data: UpdatePillRequest): Promise<Pill | null> => {
    try {
      const pill = await pillService.updatePill(id, data)
      dispatch({ type: 'UPDATE_PILL', payload: pill })
      return pill
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '更新金丹失败' })
      return null
    }
  }, [])

  /** 删除金丹 */
  const removePill = useCallback(async (id: string): Promise<boolean> => {
    try {
      await pillService.deletePill(id)
      dispatch({ type: 'REMOVE_PILL', payload: id })
      return true
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '删除金丹失败' })
      return false
    }
  }, [])

  return (
    <PillContext.Provider
      value={{
        state,
        dispatch,
        fetchPills,
        fetchPill,
        addPill,
        editPill,
        removePill,
      }}
    >
      {children}
    </PillContext.Provider>
  )
}

/** Hook */
export function usePill(): PillContextType {
  const context = useContext(PillContext)
  if (!context) {
    throw new Error('usePill must be used within a PillProvider')
  }
  return context
}

'use client'

/**
 * 对话状态管理 Context
 * 使用 React Context + useReducer 管理对话相关状态
 * 流式输出通过标准 SSE:POST /api/v1/chat/sse/:session_id(fetch + ReadableStream)
 * - 停止生成 = AbortController 中断连接,部分内容落定为「已停止」
 * - 流式中网络中断的回复标记「可能不完整」,错误消息以错误气泡内联展示
 *
 * 流式性能:
 *   - chunk 回调经 requestAnimationFrame 节流:同一帧内到达的多个 chunk 合并成一次 dispatch,
 *     React 至多以 60fps 重渲染,避免快速流时 React tree 跟不上
 *   - 流结束后由 MarkdownRenderer 自动切到完整 markdown 渲染(代码高亮等)
 */
import React, { createContext, useContext, useReducer, useCallback, useRef, useEffect } from 'react'
import * as chatService from '@/services/chatService'
import type { ChatSession, ChatMessage } from '@/services/types'

/** 对话状态 */
interface ChatState {
  sessions: ChatSession[]
  currentSession: ChatSession | null
  messages: ChatMessage[]
  loading: boolean
  streaming: boolean // 是否正在流式输出
  error: string | null
}

/** 操作类型 */
type ChatAction =
  | { type: 'SET_SESSIONS'; payload: ChatSession[] }
  | { type: 'SET_CURRENT_SESSION'; payload: ChatSession | null }
  | { type: 'SET_MESSAGES'; payload: ChatMessage[] }
  | { type: 'ADD_MESSAGE'; payload: ChatMessage }
  | { type: 'ADD_STREAM_CHUNK'; payload: string } // 追加流式输出内容
  | { type: 'FINISH_STREAM' } // 完成流式输出
  | { type: 'STOP_STREAM' } // 流式输出被停止(保留部分内容)
  | { type: 'MARK_LAST_INCOMPLETE' } // 标记最后一条助手回复「可能不完整」
  | { type: 'ADD_ERROR_MESSAGE'; payload: string } // 内联错误气泡
  | { type: 'ADD_SESSION'; payload: ChatSession }
  | { type: 'SET_LOADING'; payload: boolean }
  | { type: 'SET_STREAMING'; payload: boolean }
  | { type: 'SET_ERROR'; payload: string | null }
  | { type: 'CLEAR_CURRENT' }

/** 初始状态 */
const initialState: ChatState = {
  sessions: [],
  currentSession: null,
  messages: [],
  loading: false,
  streaming: false,
  error: null,
}

/** 将流式临时消息转换为正式消息 */
function finalizeStreamMessage(messages: ChatMessage[], patch?: Partial<ChatMessage>): ChatMessage[] {
  const result = [...messages]
  const lastMsg = result[result.length - 1]
  if (lastMsg && lastMsg.role === 'assistant' && lastMsg.id === '-1') {
    result[result.length - 1] = { ...lastMsg, id: String(Date.now()), ...patch }
  }
  return result
}

/** Reducer */
function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case 'SET_SESSIONS':
      return { ...state, sessions: action.payload, loading: false }
    case 'SET_CURRENT_SESSION':
      return { ...state, currentSession: action.payload, messages: [] }
    case 'SET_MESSAGES':
      return { ...state, messages: action.payload, loading: false }
    case 'ADD_MESSAGE':
      return { ...state, messages: [...state.messages, action.payload] }
    case 'ADD_STREAM_CHUNK': {
      // 追加到最后一条 assistant 消息,如果没有则创建
      const messages = [...state.messages]
      const lastMsg = messages[messages.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && lastMsg.id === '-1') {
        messages[messages.length - 1] = { ...lastMsg, content: lastMsg.content + action.payload }
      } else {
        messages.push({
          id: '-1', // 临时 ID
          session_id: state.currentSession?.id || '',
          role: 'assistant',
          content: action.payload,
          created_at: new Date().toISOString(),
        })
      }
      return { ...state, messages }
    }
    case 'FINISH_STREAM':
      return { ...state, messages: finalizeStreamMessage(state.messages), streaming: false }
    case 'STOP_STREAM':
      // 停止生成:保留部分内容并标记「已停止」
      return { ...state, messages: finalizeStreamMessage(state.messages, { stopped: true }), streaming: false }
    case 'MARK_LAST_INCOMPLETE': {
      const messages = [...state.messages]
      const lastMsg = messages[messages.length - 1]
      if (lastMsg && lastMsg.role === 'assistant' && !lastMsg.is_error) {
        messages[messages.length - 1] = { ...lastMsg, incomplete: true }
      }
      return { ...state, messages }
    }
    case 'ADD_ERROR_MESSAGE': {
      // 服务端错误:以错误气泡内联展示在消息流中
      const errorMessage: ChatMessage = {
        id: String(Date.now()),
        session_id: state.currentSession?.id || '',
        role: 'system',
        content: action.payload,
        created_at: new Date().toISOString(),
        is_error: true,
      }
      return { ...state, messages: [...state.messages, errorMessage], streaming: false }
    }
    case 'ADD_SESSION':
      return { ...state, sessions: [action.payload, ...state.sessions], currentSession: action.payload, loading: false }
    case 'SET_LOADING':
      return { ...state, loading: action.payload }
    case 'SET_STREAMING':
      return { ...state, streaming: action.payload }
    case 'SET_ERROR':
      return { ...state, error: action.payload }
    case 'CLEAR_CURRENT':
      return { ...state, currentSession: null, messages: [] }
    default:
      return state
  }
}

/** Context 类型 */
interface ChatContextType {
  state: ChatState
  dispatch: React.Dispatch<ChatAction>
  // 异步操作
  fetchSessions: () => Promise<void>
  createSession: (agentId: string, title?: string) => Promise<ChatSession | null>
  loadMessages: (sessionId: string) => Promise<void>
  streamMessage: (sessionId: string, content: string) => Promise<void>
  /** 停止当前流式生成(中断 SSE 连接,部分内容落定为「已停止」) */
  stopStream: () => void
}

const ChatContext = createContext<ChatContextType | null>(null)

/**
 * 流式 chunk 调度器 — requestAnimationFrame 节流
 * 同一帧内到达的多个 chunk 合并为一次 dispatch,React 至多 60fps 重渲染
 * 流结束/出错时立即 flush 剩余 buffer,保证收尾动作不延迟
 */
function createChunkDispatcher(dispatch: React.Dispatch<ChatAction>) {
  let pending = ''
  let rafId: number | null = null

  const flush = () => {
    rafId = null
    if (pending) {
      const buf = pending
      pending = ''
      dispatch({ type: 'ADD_STREAM_CHUNK', payload: buf })
    }
  }

  return {
    push(chunk: string) {
      pending += chunk
      if (rafId === null && typeof requestAnimationFrame !== 'undefined') {
        rafId = requestAnimationFrame(flush)
      } else if (rafId === null) {
        // SSR / 异常环境:直接同步 flush
        flush()
      }
    },
    /** 立即冲刷 buffer(流结束/出错时调用) */
    flushNow() {
      if (rafId !== null) {
        cancelAnimationFrame(rafId)
        rafId = null
      }
      flush()
    },
  }
}

/** Provider 组件 */
export function ChatProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(chatReducer, initialState)
  const sessionsRef = useRef<ChatSession[]>([])
  // 本轮流式是否已收到内容片段
  const partialReceivedRef = useRef(false)
  // RAF 节流的 chunk 调度器(每次 streamMessage 重新构造,避免上一轮残留)
  const chunkerRef = useRef<ReturnType<typeof createChunkDispatcher> | null>(null)

  // 同步会话列表引用,供 loadMessages 查找当前会话
  useEffect(() => {
    sessionsRef.current = state.sessions
  }, [state.sessions])

  /** 获取会话列表 */
  const fetchSessions = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const data = await chatService.listSessions()
      dispatch({ type: 'SET_SESSIONS', payload: data.list || [] })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '获取会话列表失败' })
    }
  }, [])

  /** 创建会话 */
  const createSession = useCallback(async (agentId: string, title?: string): Promise<ChatSession | null> => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      const session = await chatService.createSession({ agent_id: agentId, title })
      dispatch({ type: 'ADD_SESSION', payload: session })
      return session
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '创建会话失败' })
      return null
    }
  }, [])

  /** 加载消息历史并定位当前会话 */
  const loadMessages = useCallback(async (sessionId: string) => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      // 定位会话:先查已有列表,查不到则拉取一次会话列表
      let session = sessionsRef.current.find(s => s.id === sessionId)
      if (!session) {
        const data = await chatService.listSessions()
        dispatch({ type: 'SET_SESSIONS', payload: data.list || [] })
        session = (data.list || []).find(s => s.id === sessionId)
      }
      if (session) {
        dispatch({ type: 'SET_CURRENT_SESSION', payload: session })
      }

      const data = await chatService.getMessages(sessionId)
      dispatch({ type: 'SET_MESSAGES', payload: data.list || [] })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '加载消息失败' })
    }
  }, [])

  /** 发送消息(SSE 流式接收回复) */
  const streamMessage = useCallback(async (sessionId: string, content: string) => {
    // 先添加用户消息
    const userMessage: ChatMessage = {
      id: String(Date.now()),
      session_id: sessionId,
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    }
    dispatch({ type: 'ADD_MESSAGE', payload: userMessage })
    partialReceivedRef.current = false
    dispatch({ type: 'SET_STREAMING', payload: true })

    // 重建 chunk 调度器(确保上一轮缓冲不残留)
    const chunker = createChunkDispatcher(dispatch)
    chunkerRef.current = chunker

    await chatService.streamChatMessage(sessionId, content, {
      onChunk: (chunk) => {
        partialReceivedRef.current = true
        chunker.push(chunk)
      },
      onDone: () => {
        // 收尾:先 flush 残余 buffer,再派发完成事件
        chunker.flushNow()
        partialReceivedRef.current = false
        dispatch({ type: 'FINISH_STREAM' })
      },
      onStopped: () => {
        chunker.flushNow()
        partialReceivedRef.current = false
        dispatch({ type: 'STOP_STREAM' })
      },
      onError: (error) => {
        chunker.flushNow()
        partialReceivedRef.current = false
        dispatch({ type: 'ADD_ERROR_MESSAGE', payload: error })
      },
      onInterrupted: () => {
        // 流式生成中网络中断:恢复输入,标记该条回复「可能不完整」
        chunker.flushNow()
        partialReceivedRef.current = false
        dispatch({ type: 'FINISH_STREAM' })
        dispatch({ type: 'MARK_LAST_INCOMPLETE' })
      },
    })
  }, [])

  /** 停止当前流式生成(中断连接,服务端保存部分内容) */
  const stopStream = useCallback(() => {
    chatService.stopStream()
  }, [])

  return (
    <ChatContext.Provider
      value={{
        state,
        dispatch,
        fetchSessions,
        createSession,
        loadMessages,
        streamMessage,
        stopStream,
      }}
    >
      {children}
    </ChatContext.Provider>
  )
}

/** Hook */
export function useChat(): ChatContextType {
  const context = useContext(ChatContext)
  if (!context) {
    throw new Error('useChat must be used within a ChatProvider')
  }
  return context
}

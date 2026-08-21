'use client'

/**
 * 对话状态管理 Context
 * 使用 React Context + useReducer 管理对话相关状态
 * 流式输出通过标准 SSE:POST /api/v1/chat/sse/:session_id(fetch + ReadableStream)
 * - 停止生成 = AbortController 中断连接,部分内容落定为「已停止」
 * - 流式中网络中断的回复标记「可能不完整」,错误消息以错误气泡内联展示
 *
 * 流式性能:按 SSE 到达顺序直接交付，不做人工逐字队列；结束事件统一收拢状态。
 */
import React, { createContext, useContext, useReducer, useCallback, useRef, useEffect } from 'react'
import * as chatService from '@/services/chatService'
import type { StreamChunk, StreamSpeakerInfo } from '@/services/chatService'
import { createStreamDispatcher } from '@/services/streamDispatcher'
import { notifyDesktop } from '@/services/api'
import type { ChatSession, ChatMessage } from '@/services/types'

/** 对话状态 */
interface ChatState {
  sessions: ChatSession[]
  currentSession: ChatSession | null
  messages: ChatMessage[]
  loading: boolean
  streaming: boolean // 是否正在流式输出
  error: string | null
  /** 会话列表加载错误；与创建、消息等操作错误分开归属。 */
  sessionsError: string | null
  /** 当前 URL 会话的独立加载生命周期，不与列表/创建错误共享。 */
  sessionLoad: {
    status: 'idle' | 'loading' | 'ready' | 'not-found' | 'error'
    message?: string
  }
  /** 群聊: 当前正在发言的道人(用于 typing 指示器显示名字/头像) */
  currentSpeaker: { agent_id: string; agent_name: string; agent_avatar?: string } | null
}

/** 操作类型 */
type ChatAction =
  | { type: 'SET_SESSIONS'; payload: ChatSession[] }
  | { type: 'SET_CURRENT_SESSION'; payload: ChatSession | null }
  | { type: 'SET_MESSAGES'; payload: ChatMessage[] }
  | { type: 'ADD_MESSAGE'; payload: ChatMessage }
  | { type: 'ADD_STREAM_CHUNK'; payload: StreamChunk } // 追加流式输出内容
  | { type: 'FINISH_STREAM' }
  | { type: 'FINALIZE_STREAM' }
  | { type: 'STOP_STREAM' } // 流式输出被停止(保留部分内容)
  | { type: 'MARK_STREAM_INCOMPLETE'; payload: { agent_id?: string; retryable: boolean } }
  | { type: 'DISCARD_EMPTY_STREAM'; payload?: { agent_id?: string } }
  | { type: 'ADD_ERROR_MESSAGE'; payload: { text: string; retryable: boolean } }
  /** 回合内系统通知(群聊单道人失败等): 追加系统条,不动 streaming 状态 */
  | { type: 'ADD_SYSTEM_NOTICE'; payload: { text: string; isError: boolean; retryable?: boolean } }
  | { type: 'SPEAKER_START'; payload: { agent_id: string; agent_name: string; agent_avatar?: string } }
  | { type: 'SPEAKER_DONE'; payload: StreamSpeakerInfo }
  /** 群聊:用服务端真实 message_id 替换本地临时 id，可附 mentions */
  | { type: 'FINALIZE_STREAM_WITH_ID'; payload: StreamSpeakerInfo & { message_id: string; mentions?: import('@/services/types').ChatMessage['mentions'] } }
  | { type: 'SET_SESSION_TITLE'; payload: { sessionId: string; title: string } }
  | { type: 'UPDATE_SESSION_MEMBERS'; payload: { sessionId: string; members: import('@/services/types').GroupMember[] } }
  | { type: 'ADD_SESSION'; payload: ChatSession }
  | { type: 'UPSERT_SESSION'; payload: ChatSession }
  | { type: 'SET_LOADING'; payload: boolean }
  | { type: 'SET_STREAMING'; payload: boolean }
  | { type: 'SET_ERROR'; payload: string | null }
  | { type: 'SET_SESSIONS_ERROR'; payload: string | null }
  | { type: 'SESSION_LOAD_START' }
  | { type: 'SESSION_LOAD_READY'; payload: { session: ChatSession; messages: ChatMessage[] } }
  | { type: 'SESSION_LOAD_NOT_FOUND' }
  | { type: 'SESSION_LOAD_ERROR'; payload: string }
  | { type: 'CLEAR_CURRENT' }

/** 初始状态 */
const initialState: ChatState = {
  sessions: [],
  currentSession: null,
  messages: [],
  loading: false,
  streaming: false,
  error: null,
  sessionsError: null,
  sessionLoad: { status: 'idle' },
  currentSpeaker: null,
}

let localMessageSequence = 0
const localMessageID = (prefix: string) => `${prefix}-${Date.now()}-${++localMessageSequence}`
const streamMessageID = () => localMessageID('stream')
const isStreamMessage = (message?: ChatMessage) => Boolean(message?.id.startsWith('stream-'))

/** 将当前流式临时消息转换为正式消息 */
function findStreamMessageIndex(messages: ChatMessage[], agentId?: string): number {
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const message = messages[i]
    if (message.role !== 'assistant' || !isStreamMessage(message)) continue
    if (!agentId || message.agent_id === agentId) return i
  }
  return -1
}

function finalizeStreamMessage(messages: ChatMessage[], patch?: Partial<ChatMessage>, agentId?: string): ChatMessage[] {
  const result = [...messages]
  const index = findStreamMessageIndex(result, agentId)
  if (index >= 0) {
    result[index] = { ...result[index], id: localMessageID('assistant'), ...patch }
  }
  return result
}

/** Reducer */
function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case 'SET_SESSIONS':
      return { ...state, sessions: action.payload, sessionsError: null, loading: false }
    case 'SET_CURRENT_SESSION':
      return { ...state, currentSession: action.payload, messages: [] }
    case 'SET_MESSAGES':
      return { ...state, messages: action.payload, loading: false }
    case 'ADD_MESSAGE':
      return { ...state, messages: [...state.messages, action.payload] }
    case 'ADD_STREAM_CHUNK': {
      // 群聊按 chunk 自带身份定位气泡；单聊无身份时使用当前临时回复。
      const messages = [...state.messages]
      const index = findStreamMessageIndex(messages, action.payload.agent_id)
      if (index >= 0) {
        const current = messages[index]
        messages[index] = {
          ...current,
          content: current.content + action.payload.content,
          agent_id: action.payload.agent_id || current.agent_id,
          agent_name: action.payload.agent_name || current.agent_name,
          agent_avatar: action.payload.agent_avatar || current.agent_avatar,
        }
      } else {
        messages.push({
          id: streamMessageID(),
          session_id: state.currentSession?.id || '',
          role: 'assistant',
          content: action.payload.content,
          agent_id: action.payload.agent_id,
          agent_name: action.payload.agent_name,
          agent_avatar: action.payload.agent_avatar,
          created_at: new Date().toISOString(),
        })
      }
      return { ...state, messages }
    }
    case 'FINISH_STREAM':
      return { ...state, streaming: false }
    case 'FINALIZE_STREAM':
      return { ...state, messages: finalizeStreamMessage(state.messages) }
    case 'STOP_STREAM':
      // 停止生成:保留部分内容并标记「已停止」
      return { ...state, messages: finalizeStreamMessage(state.messages, { stopped: true }), streaming: false }
    case 'MARK_STREAM_INCOMPLETE': {
      const messages = finalizeStreamMessage(
        state.messages,
        { incomplete: true, retryable: action.payload.retryable },
        action.payload.agent_id,
      )
      return { ...state, messages }
    }
    case 'DISCARD_EMPTY_STREAM': {
      const messages = [...state.messages]
      const index = findStreamMessageIndex(messages, action.payload?.agent_id)
      if (index >= 0 && messages[index].content === '') {
        messages.splice(index, 1)
      }
      return { ...state, messages, currentSpeaker: null }
    }
    case 'ADD_ERROR_MESSAGE': {
      // 服务端错误:以错误气泡内联展示在消息流中
      const errorMessage: ChatMessage = {
        id: localMessageID('error'),
        session_id: state.currentSession?.id || '',
        role: 'system',
        content: action.payload.text,
        created_at: new Date().toISOString(),
        is_error: true,
        retryable: action.payload.retryable,
      }
      return { ...state, messages: [...state.messages, errorMessage], streaming: false }
    }
    case 'ADD_SYSTEM_NOTICE': {
      // 回合内通知(群聊单道人失败等): 按序插入系统条,回合流式状态保持不变
      const notice: ChatMessage = {
        id: localMessageID('notice'),
        session_id: state.currentSession?.id || '',
        role: 'system',
        content: action.payload.text,
        created_at: new Date().toISOString(),
        is_error: action.payload.isError,
        retryable: action.payload.retryable,
      }
      return { ...state, messages: [...state.messages, notice] }
    }
    case 'SPEAKER_START': {
      // 群聊:每位道人使用独立临时 ID，避免 React key 重复导致气泡错位。
      const messages = finalizeStreamMessage(state.messages)
      messages.push({
        id: streamMessageID(),
        session_id: state.currentSession?.id || '',
        role: 'assistant',
        content: '',
        agent_id: action.payload.agent_id,
        agent_name: action.payload.agent_name,
        agent_avatar: action.payload.agent_avatar,
        created_at: new Date().toISOString(),
      })
      return { ...state, messages, currentSpeaker: action.payload }
    }
    case 'SPEAKER_DONE':
      return {
        ...state,
        messages: finalizeStreamMessage(state.messages, undefined, action.payload.agent_id),
        currentSpeaker: state.currentSpeaker?.agent_id === action.payload.agent_id ? null : state.currentSpeaker,
      }
    case 'FINALIZE_STREAM_WITH_ID': {
      // 群聊:服务端已返回真实 message_id，替换当前临时 id。
      const messages = [...state.messages]
      const index = findStreamMessageIndex(messages, action.payload.agent_id)
      if (index >= 0) {
        messages[index] = {
          ...messages[index],
          id: action.payload.message_id,
          agent_id: action.payload.agent_id,
          agent_name: action.payload.agent_name,
          agent_avatar: action.payload.agent_avatar,
          mentions: action.payload.mentions || messages[index].mentions,
        }
      }
      return {
        ...state,
        messages,
        currentSpeaker: state.currentSpeaker?.agent_id === action.payload.agent_id ? null : state.currentSpeaker,
      }
    }
    case 'SET_SESSION_TITLE': {
      const { sessionId, title } = action.payload
      const sessions = state.sessions.map(s => s.id === sessionId ? { ...s, title } : s)
      const currentSession = state.currentSession?.id === sessionId ? { ...state.currentSession, title } : state.currentSession
      return { ...state, sessions, currentSession }
    }
    case 'UPDATE_SESSION_MEMBERS': {
      const { sessionId, members } = action.payload
      const sessions = state.sessions.map(s => s.id === sessionId ? { ...s, members } : s)
      const currentSession = state.currentSession?.id === sessionId ? { ...state.currentSession, members } : state.currentSession
      return { ...state, sessions, currentSession }
    }
    case 'ADD_SESSION':
      return { ...state, sessions: [action.payload, ...state.sessions], currentSession: action.payload, loading: false }
    case 'UPSERT_SESSION':
      return {
        ...state,
        sessions: [action.payload, ...state.sessions.filter(session => session.id !== action.payload.id)],
      }
    case 'SET_LOADING':
      return { ...state, loading: action.payload }
    case 'SET_STREAMING':
      return { ...state, streaming: action.payload }
    case 'SET_ERROR':
      return { ...state, error: action.payload, loading: false }
    case 'SET_SESSIONS_ERROR':
      return { ...state, sessionsError: action.payload, loading: false }
    case 'SESSION_LOAD_START':
      return {
        ...state,
        currentSession: null,
        messages: [],
        loading: true,
        sessionLoad: { status: 'loading' },
      }
    case 'SESSION_LOAD_READY':
      return {
        ...state,
        currentSession: action.payload.session,
        messages: action.payload.messages,
        loading: false,
        sessionLoad: { status: 'ready' },
      }
    case 'SESSION_LOAD_NOT_FOUND':
      return {
        ...state,
        currentSession: null,
        messages: [],
        loading: false,
        sessionLoad: { status: 'not-found' },
      }
    case 'SESSION_LOAD_ERROR':
      return {
        ...state,
        currentSession: null,
        messages: [],
        loading: false,
        sessionLoad: { status: 'error', message: action.payload },
      }
    case 'CLEAR_CURRENT':
      return { ...state, currentSession: null, messages: [], loading: false, sessionLoad: { status: 'idle' } }
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
  createSession: (agentId: string, title?: string) => Promise<ChatSession>
  createGroupSession: (memberAgentIds: string[]) => Promise<ChatSession>
  renameSession: (sessionId: string, title: string) => Promise<ChatSession | null>
  inviteMembers: (sessionId: string, agentIds: string[]) => Promise<void>
  kickMember: (sessionId: string, agentId: string) => Promise<void>
  loadMessages: (sessionId: string) => Promise<void>
  clearCurrent: () => void
  streamMessage: (sessionId: string, content: string, opts?: {
    retry?: boolean
    retryBoundaryText?: string
    interruptedText?: string
  }) => Promise<void>
  /** 停止当前流式生成(中断 SSE 连接,部分内容落定为「已停止」) */
  stopStream: () => void
}

const ChatContext = createContext<ChatContextType | null>(null)

/**
 * 流式调度器按 SSE 原序同步交付，不制造额外打字延迟。
 */

/** Provider 组件 */
export function ChatProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(chatReducer, initialState)
  const sessionsRef = useRef<ChatSession[]>([])
  const sessionLoadRequestRef = useRef(0)

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
      dispatch({
        type: 'SET_SESSIONS_ERROR',
        payload: error instanceof Error ? error.message : '获取会话列表失败',
      })
    }
  }, [])

  /** 创建 1v1 会话 */
  const createSession = useCallback(async (agentId: string, title?: string): Promise<ChatSession> => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      // title 参数保留但忽略(后端自动命名)
      const session = await chatService.createSession({ agent_id: agentId, title })
      dispatch({ type: 'ADD_SESSION', payload: session })
      return session
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '创建会话失败' })
      throw error
    }
  }, [])

  /** 建群(≥2 位道人;首问答自动命名) */
  const createGroupSession = useCallback(async (memberAgentIds: string[]): Promise<ChatSession> => {
    try {
      const session = await chatService.createGroupSession(memberAgentIds)
      dispatch({ type: 'ADD_SESSION', payload: session })
      return session
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '建群失败' })
      throw error
    }
  }, [])

  /** 重命名会话 */
  const renameSession = useCallback(async (sessionId: string, title: string): Promise<ChatSession | null> => {
    try {
      const session = await chatService.renameSession(sessionId, title)
      dispatch({ type: 'SET_SESSION_TITLE', payload: { sessionId, title: session.title || title } })
      return session
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '改名失败' })
      return null
    }
  }, [])

  /** 邀请入群(已在群静默跳过);更新本地 members */
  const inviteMembers = useCallback(async (sessionId: string, agentIds: string[]) => {
    try {
      const { members } = await chatService.addMembers(sessionId, agentIds)
      dispatch({ type: 'UPDATE_SESSION_MEMBERS', payload: { sessionId, members } })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '邀请失败' })
    }
  }, [])

  /** 移出群成员;更新本地 members */
  const kickMember = useCallback(async (sessionId: string, agentId: string) => {
    try {
      const { members } = await chatService.removeMember(sessionId, agentId)
      dispatch({ type: 'UPDATE_SESSION_MEMBERS', payload: { sessionId, members } })
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '踢人失败' })
    }
  }, [])

  /** 加载消息历史并定位当前会话 */
  const loadMessages = useCallback(async (sessionId: string) => {
    const requestId = ++sessionLoadRequestRef.current
    dispatch({ type: 'SESSION_LOAD_START' })
    try {
      // 定位会话:先查已有列表,查不到则按 UUID 直取；不受列表 100 条上限影响。
      let session = sessionsRef.current.find(s => s.id === sessionId)
      if (!session) {
        session = await chatService.getSession(sessionId)
        if (requestId !== sessionLoadRequestRef.current) return
        sessionsRef.current = [session, ...sessionsRef.current.filter(item => item.id !== sessionId)]
        dispatch({ type: 'UPSERT_SESSION', payload: session })
      }

      const data = await chatService.getMessages(sessionId)
      if (requestId !== sessionLoadRequestRef.current) return
      dispatch({ type: 'SESSION_LOAD_READY', payload: { session, messages: data.list || [] } })
    } catch (error) {
      if (requestId !== sessionLoadRequestRef.current) return
      const status = typeof error === 'object' && error !== null && 'status' in error
        ? Number((error as { status?: unknown }).status)
        : 0
      if (status === 404) {
        dispatch({ type: 'SESSION_LOAD_NOT_FOUND' })
      } else {
        dispatch({ type: 'SESSION_LOAD_ERROR', payload: error instanceof Error ? error.message : '加载消息失败' })
      }
    }
  }, [])

  /** 离开会话时同时让所有在途元数据/历史响应失效。 */
  const clearCurrent = useCallback(() => {
    sessionLoadRequestRef.current += 1
    dispatch({ type: 'CLEAR_CURRENT' })
  }, [])

  /** 发送或重试消息(SSE 流式接收回复)。重试不追加本地用户气泡。 */
  const streamMessage = useCallback(async (
    sessionId: string,
    content: string,
    opts?: { retry?: boolean; retryBoundaryText?: string; interruptedText?: string },
  ) => {
    const isGroup = sessionsRef.current.find(s => s.id === sessionId)?.type === 'group'
    if (!opts?.retry) {
      dispatch({
        type: 'ADD_MESSAGE',
        payload: {
          id: localMessageID('user'),
          session_id: sessionId,
          role: 'user',
          content,
          created_at: new Date().toISOString(),
        },
      })
    } else if (isGroup && opts.retryBoundaryText) {
      dispatch({ type: 'ADD_SYSTEM_NOTICE', payload: { text: opts.retryBoundaryText, isError: false } })
    }

    // 每个请求独立持有部分内容/当前发言人/终止状态；迟到的旧请求回调不能污染新回合。
    let turnPartial = false
    let speakerPartial = false
    let activeSpeaker: StreamSpeakerInfo | null = null
    let turnTerminated = false
    dispatch({ type: 'SET_STREAMING', payload: true })

    const chunker = createStreamDispatcher({
      onChunk: (chunk) => dispatch({ type: 'ADD_STREAM_CHUNK', payload: chunk }),
      onSpeakerStart: (info) => dispatch({ type: 'SPEAKER_START', payload: info }),
      onSpeakerDone: (info) => {
        if (info.message_id) {
          dispatch({
            type: 'FINALIZE_STREAM_WITH_ID',
            payload: { ...info, message_id: info.message_id, mentions: info.mentions as ChatMessage['mentions'] },
          })
        } else {
          dispatch({ type: 'SPEAKER_DONE', payload: info })
        }
      },
      onNotice: (text, isError, retryable) => dispatch({
        type: 'ADD_SYSTEM_NOTICE',
        payload: { text, isError, retryable },
      }),
      onDrained: () => {
        dispatch({ type: 'FINALIZE_STREAM' })
        dispatch({ type: 'FINISH_STREAM' })
      },
    })
    const finishTerminal = (errorText: string) => {
      if (turnTerminated) return
      turnTerminated = true
      chunker.flushNow()
      const active = activeSpeaker
      if (isGroup) {
        if (active) {
          if (speakerPartial) {
            dispatch({ type: 'MARK_STREAM_INCOMPLETE', payload: { agent_id: active.agent_id, retryable: false } })
          } else {
            dispatch({ type: 'DISCARD_EMPTY_STREAM', payload: { agent_id: active.agent_id } })
          }
        }
        dispatch({ type: 'ADD_ERROR_MESSAGE', payload: { text: errorText, retryable: true } })
      } else if (turnPartial) {
        dispatch({ type: 'MARK_STREAM_INCOMPLETE', payload: { retryable: true } })
        dispatch({ type: 'FINISH_STREAM' })
      } else {
        dispatch({ type: 'ADD_ERROR_MESSAGE', payload: { text: errorText, retryable: true } })
      }
      activeSpeaker = null
      speakerPartial = false
      turnPartial = false
    }

    await chatService.streamChatMessage(sessionId, content, {
      onChunk: (chunk) => {
        turnPartial = true
        if (isGroup) {
          speakerPartial = true
          if (chunk.agent_id) {
            activeSpeaker = {
              agent_id: chunk.agent_id,
              agent_name: chunk.agent_name || activeSpeaker?.agent_name || '',
              agent_avatar: chunk.agent_avatar || activeSpeaker?.agent_avatar,
            }
          }
        }
        chunker.pushChunk(chunk)
      },
      onDone: () => {
        if (turnTerminated) return
        turnTerminated = true
        turnPartial = false
        chunker.markDone()
        notifyDesktop()
      },
      onStopped: () => {
        if (turnTerminated) return
        turnTerminated = true
        chunker.flushNow()
        activeSpeaker = null
        speakerPartial = false
        turnPartial = false
        dispatch({ type: 'STOP_STREAM' })
      },
      onError: (error, info) => {
        if (info.terminal || !isGroup) {
          finishTerminal(error)
          return
        }
        const active = activeSpeaker
        if (active && (!info.agent_id || info.agent_id === active.agent_id)) {
          if (speakerPartial) {
            dispatch({ type: 'MARK_STREAM_INCOMPLETE', payload: { agent_id: active.agent_id, retryable: false } })
          } else {
            dispatch({ type: 'DISCARD_EMPTY_STREAM', payload: { agent_id: active.agent_id } })
          }
          activeSpeaker = null
          speakerPartial = false
        }
        chunker.pushNotice(error, true, false)
      },
      onInterrupted: () => finishTerminal(opts?.interruptedText || ''),
      onSpeakerStart: (info) => {
        activeSpeaker = info
        speakerPartial = false
        chunker.pushSpeakerStart(info)
      },
      onSpeakerDone: (info) => {
        chunker.pushSpeakerDone(info)
        if (activeSpeaker?.agent_id === info.agent_id) {
          activeSpeaker = null
          speakerPartial = false
        }
      },
      onTurnDone: () => {
        if (turnTerminated) return
        turnTerminated = true
        activeSpeaker = null
        speakerPartial = false
        turnPartial = false
        chunker.markDone()
        notifyDesktop()
      },
      onTitle: (title) => dispatch({ type: 'SET_SESSION_TITLE', payload: { sessionId, title } }),
    }, { retry: opts?.retry })
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
        createGroupSession,
        renameSession,
        inviteMembers,
        kickMember,
        loadMessages,
        clearCurrent,
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

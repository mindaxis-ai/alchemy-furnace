'use client'

/**
 * 对话状态管理 Context
 * 使用 React Context + useReducer 管理对话相关状态
 * 流式输出通过标准 SSE:POST /api/v1/chat/sse/:session_id(fetch + ReadableStream)
 * - 停止生成 = AbortController 中断连接,部分内容落定为「已停止」
 * - 流式中网络中断的回复标记「可能不完整」,错误消息以错误气泡内联展示
 *
 * 流式性能:
 *   - chunk 入 createChunkDispatcher 队列后,以 30ms 节奏依次派发(见 createChunkDispatcher)
 *     即使 LLM 服务端把整段一次性 flush,前端也以"打字机"节奏逐 chunk 渲染,体感如 deepseek
 *   - 流结束后由 MarkdownRenderer 自动切到完整 markdown 渲染(代码高亮等)
 */
import React, { createContext, useContext, useReducer, useCallback, useRef, useEffect } from 'react'
import * as chatService from '@/services/chatService'
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
  /** 群聊: 当前正在发言的道人(用于 typing 指示器显示名字/头像) */
  currentSpeaker: { agent_id: string; agent_name: string; agent_avatar?: string } | null
}

/** 操作类型 */
type ChatAction =
  | { type: 'SET_SESSIONS'; payload: ChatSession[] }
  | { type: 'SET_CURRENT_SESSION'; payload: ChatSession | null }
  | { type: 'SET_MESSAGES'; payload: ChatMessage[] }
  | { type: 'ADD_MESSAGE'; payload: ChatMessage }
  | { type: 'ADD_STREAM_CHUNK'; payload: string } // 追加流式输出内容
  | { type: 'FINISH_STREAM' } // 完成流式输出(仅标记,不 finalize,等 queue 排空)
  | { type: 'FINALIZE_STREAM' } // typing queue 已清空,正式 finalize 临时消息
  | { type: 'STOP_STREAM' } // 流式输出被停止(保留部分内容)
  | { type: 'MARK_LAST_INCOMPLETE' } // 标记最后一条助手回复「可能不完整」
  | { type: 'ADD_ERROR_MESSAGE'; payload: string } // 内联错误气泡
  /** 回合内系统通知(群聊单道人失败等): 追加系统条,不动 streaming 状态 */
  | { type: 'ADD_SYSTEM_NOTICE'; payload: { text: string; isError: boolean } }
  | { type: 'SPEAKER_START'; payload: { agent_id: string; agent_name: string; agent_avatar?: string } }
  | { type: 'SPEAKER_DONE' }
  /** 群聊:用服务端真实 message_id 替换临时 id='-1',可附 mentions */
  | { type: 'FINALIZE_STREAM_WITH_ID'; payload: { message_id: string; mentions?: import('@/services/types').ChatMessage['mentions'] } }
  | { type: 'SET_SESSION_TITLE'; payload: { sessionId: string; title: string } }
  | { type: 'UPDATE_SESSION_MEMBERS'; payload: { sessionId: string; members: import('@/services/types').GroupMember[] } }
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
  currentSpeaker: null,
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

/**
 * typing 队列已清空,流式消息可以 finalize(把 id='-1' 改为真实 id)
 * 这是关键修复: 不能 FINISH_STREAM 立即 finalize,否则 typing queue 里残余 chunk
 * 继续派发时会找不到 id='-1' 的消息,创建一条新的 '-1' 临时消息,UI 出现两条回复。
 */
function finalizeStreamMessageWhenQueueEmpty(messages: ChatMessage[], patch?: Partial<ChatMessage>): ChatMessage[] {
  return finalizeStreamMessage(messages, patch)
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
      // 仅停止流式标记,不 finalize — typing queue 可能仍有未派发 chunk
      return { ...state, streaming: false }
    case 'FINALIZE_STREAM':
      // typing queue 已清空,正式 finalize 临时消息(把 id='-1' 改为真实 id)
      return { ...state, messages: finalizeStreamMessageWhenQueueEmpty(state.messages) }
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
    case 'ADD_SYSTEM_NOTICE': {
      // 回合内通知(群聊单道人失败等): 按序插入系统条,回合流式状态保持不变
      const notice: ChatMessage = {
        id: String(Date.now()),
        session_id: state.currentSession?.id || '',
        role: 'system',
        content: action.payload.text,
        created_at: new Date().toISOString(),
        is_error: action.payload.isError,
      }
      return { ...state, messages: [...state.messages, notice] }
    }
    case 'SPEAKER_START': {
      // 群聊: 某道人开始发言,先把可能的 -1 临时消息 finalize,再开新气泡
      const messages = finalizeStreamMessage(state.messages)
      messages.push({
        id: '-1',
        session_id: state.currentSession?.id || '',
        role: 'assistant',
        content: '',
        agent_id: action.payload.agent_id,
        agent_name: action.payload.agent_name,
        created_at: new Date().toISOString(),
      })
      return { ...state, messages, currentSpeaker: { agent_id: action.payload.agent_id, agent_name: action.payload.agent_name } }
    }
    case 'SPEAKER_DONE':
      // 群聊: 当前发言人 finalize,清空 currentSpeaker
      return { ...state, messages: finalizeStreamMessage(state.messages), currentSpeaker: null }
    case 'FINALIZE_STREAM_WITH_ID': {
      // 群聊: 服务端已返回真实 message_id,直接用该 id 替换 -1(同时携带 mentions)
      const messages = [...state.messages]
      const lastIdx = messages.length - 1
      if (lastIdx >= 0 && messages[lastIdx].role === 'assistant' && messages[lastIdx].id === '-1') {
        messages[lastIdx] = {
          ...messages[lastIdx],
          id: action.payload.message_id,
          mentions: action.payload.mentions || messages[lastIdx].mentions,
        }
      }
      return { ...state, messages, currentSpeaker: null }
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
  createGroupSession: (memberAgentIds: string[]) => Promise<ChatSession | null>
  renameSession: (sessionId: string, title: string) => Promise<ChatSession | null>
  inviteMembers: (sessionId: string, agentIds: string[]) => Promise<void>
  kickMember: (sessionId: string, agentId: string) => Promise<void>
  loadMessages: (sessionId: string) => Promise<void>
  streamMessage: (sessionId: string, content: string, opts?: { allSilentText?: string }) => Promise<void>
  /** 停止当前流式生成(中断 SSE 连接,部分内容落定为「已停止」) */
  stopStream: () => void
}

const ChatContext = createContext<ChatContextType | null>(null)

/**
 * 流式调度器已抽至 @/services/streamDispatcher(群聊标记队列版)
 * 关键差异: 发言人切换事件与 chunk 同队列按序消费,不再 flush 清仓,
 * 修复群聊"一次性加载"(详见该文件头注释与 scripts/test-stream-dispatcher.ts)
 */

/** Provider 组件 */
export function ChatProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(chatReducer, initialState)
  const sessionsRef = useRef<ChatSession[]>([])
  // 本轮流式是否已收到内容片段
  const partialReceivedRef = useRef(false)
  // 群聊: 本回合是否已有发言人开腔(用于区分单道人失败 vs 启动即败的致命错误)
  const groupActiveRef = useRef(false)
  // 流式事件调度器(每次 streamMessage 重新构造,避免上一轮残留)
  const chunkerRef = useRef<ReturnType<typeof createStreamDispatcher> | null>(null)

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

  /** 创建 1v1 会话 */
  const createSession = useCallback(async (agentId: string, title?: string): Promise<ChatSession | null> => {
    dispatch({ type: 'SET_LOADING', payload: true })
    try {
      // title 参数保留但忽略(后端自动命名)
      const session = await chatService.createSession({ agent_id: agentId, title })
      dispatch({ type: 'ADD_SESSION', payload: session })
      return session
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '创建会话失败' })
      return null
    }
  }, [])

  /** 建群(≥2 位道人;首问答自动命名) */
  const createGroupSession = useCallback(async (memberAgentIds: string[]): Promise<ChatSession | null> => {
    try {
      const session = await chatService.createGroupSession(memberAgentIds)
      dispatch({ type: 'ADD_SESSION', payload: session })
      return session
    } catch (error) {
      dispatch({ type: 'SET_ERROR', payload: error instanceof Error ? error.message : '建群失败' })
      return null
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

    // 重建流式调度器(发言人标记与 chunk 同队列按序消费,切换不再 flush)
    // onDrained: typing 队列清空时 finalize 临时消息(把 id='-1' 改为真实 id)
    const isGroup = sessionsRef.current.find(s => s.id === sessionId)?.type === 'group'
    groupActiveRef.current = false
    const chunker = createStreamDispatcher({
      onChunk: (text) => dispatch({ type: 'ADD_STREAM_CHUNK', payload: text }),
      onSpeakerStart: (info) => dispatch({ type: 'SPEAKER_START', payload: info }),
      onSpeakerDone: (info) => {
        if (info.message_id) {
          dispatch({ type: 'FINALIZE_STREAM_WITH_ID', payload: { message_id: info.message_id, mentions: info.mentions as ChatMessage['mentions'] } })
        } else {
          dispatch({ type: 'SPEAKER_DONE' })
        }
      },
      onNotice: (text, isError) => dispatch({ type: 'ADD_SYSTEM_NOTICE', payload: { text, isError } }),
      onDrained: () => dispatch({ type: 'FINALIZE_STREAM' }),
    })
    chunkerRef.current = chunker

    await chatService.streamChatMessage(sessionId, content, {
      onChunk: (chunk) => {
        partialReceivedRef.current = true
        chunker.pushChunk(chunk)
      },
      onDone: () => {
        // 单聊: 服务端 [DONE] → 标记结束,typing 按节奏自然排空后由 onDrained finalize
        partialReceivedRef.current = false
        chunker.markDone()
        dispatch({ type: 'FINISH_STREAM' })
        // T6: 回合完成提醒(窗口未聚焦时 Dock 弹跳)
        notifyDesktop()
      },
      onStopped: () => {
        // 用户主动停止: 按序立即应用队列残余,STOP_STREAM 给 -1 临时消息打「已停止」
        chunker.flushNow()
        partialReceivedRef.current = false
        groupActiveRef.current = false
        dispatch({ type: 'STOP_STREAM' })
      },
      onError: (error) => {
        // 群聊回合中(已有发言人/内容): 单道人失败按序插通知条,回合继续
        // 否则(单聊/群聊启动即败,后端不再发 turn_done): 致命错误,flush 收尾
        if (isGroup && (groupActiveRef.current || partialReceivedRef.current)) {
          chunker.pushNotice(error, true)
          return
        }
        chunker.flushNow()
        partialReceivedRef.current = false
        groupActiveRef.current = false
        dispatch({ type: 'ADD_ERROR_MESSAGE', payload: error })
      },
      onInterrupted: () => {
        // 流式生成中网络中断: 按序立即应用残余内容
        chunker.flushNow()
        partialReceivedRef.current = false
        groupActiveRef.current = false
        dispatch({ type: 'FINISH_STREAM' })
        dispatch({ type: 'MARK_LAST_INCOMPLETE' })
      },
      // ========== 群聊回调(标记入队,不打断 typing 节奏) ==========
      onSpeakerStart: (info) => {
        groupActiveRef.current = true
        chunker.pushSpeakerStart(info)
      },
      onSpeakerDone: (info) => {
        chunker.pushSpeakerDone(info)
      },
      onTurnDone: () => {
        // 回合结束: 标记服务端完毕,队列按节奏播完由 onDrained finalize
        partialReceivedRef.current = false
        groupActiveRef.current = false
        chunker.markDone()
        dispatch({ type: 'FINISH_STREAM' })
        // T6: 群聊回合完成提醒
        notifyDesktop()
      },
      onTitle: (title) => {
        // 自动命名(首问答命名)
        dispatch({ type: 'SET_SESSION_TITLE', payload: { sessionId, title } })
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
        createGroupSession,
        renameSession,
        inviteMembers,
        kickMember,
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

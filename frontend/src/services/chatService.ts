/**
 * 对话服务 - 会话管理和消息 API
 * 对接后端 /api/v1/chat/sessions 与 WebSocket /api/v1/chat/ws/:session_id
 * 无 RAG 引用来源（sources 已移除）
 */
import { get, post, buildWsUrl } from './api'
import type { ChatSession, ChatMessage, CreateSessionRequest, PagedList, ListParams } from './types'

/**
 * 获取会话列表
 */
export function listSessions(params: ListParams = {}): Promise<PagedList<ChatSession>> {
  return get<PagedList<ChatSession>>('/chat/sessions', {
    page: params.page ?? 1,
    page_size: params.page_size ?? 100,
  })
}

/**
 * 创建会话
 */
export function createSession(data: CreateSessionRequest): Promise<ChatSession> {
  return post<ChatSession>('/chat/sessions', data)
}

/**
 * 获取会话消息历史（按时间正序）
 */
export function getMessages(sessionId: number, params: ListParams = {}): Promise<PagedList<ChatMessage>> {
  return get<PagedList<ChatMessage>>(`/chat/sessions/${sessionId}/messages`, {
    page: params.page ?? 1,
    page_size: params.page_size ?? 200,
  })
}

/** 流式对话回调 */
export interface StreamHandlers {
  /** 收到内容片段 */
  onChunk: (content: string) => void
  /** 流式输出完成 */
  onDone: () => void
  /** 发生错误 */
  onError: (error: string) => void
}

// 当前活跃的 WebSocket 连接（同一时间只允许一条流式输出）
let activeSocket: WebSocket | null = null

/**
 * 通过 WebSocket 发送消息并流式接收回复
 * 服务端协议: 客户端发送 { content }，服务端推送 { type: chunk|done|error, content }
 * @returns 取消函数（关闭连接）
 */
export function streamChatMessage(
  sessionId: number,
  content: string,
  handlers: StreamHandlers
): () => void {
  // 关闭已有连接
  cancelStream()

  const ws = new WebSocket(buildWsUrl(`/chat/ws/${sessionId}`))
  activeSocket = ws
  let finished = false

  const finish = () => {
    if (finished) return
    finished = true
    if (activeSocket === ws) activeSocket = null
    ws.close()
  }

  ws.onopen = () => {
    ws.send(JSON.stringify({ content }))
  }

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data as string) as { type: string; content?: string }
      if (msg.type === 'chunk' && msg.content) {
        handlers.onChunk(msg.content)
      } else if (msg.type === 'done') {
        finish()
        handlers.onDone()
      } else if (msg.type === 'error') {
        finish()
        handlers.onError(msg.content || '论道出错了')
      }
    } catch {
      // 忽略无法解析的消息
    }
  }

  ws.onerror = () => {
    finish()
    handlers.onError('WebSocket 连接失败，请确认炼丹服务已启动')
  }

  ws.onclose = () => {
    if (activeSocket === ws) activeSocket = null
  }

  return finish
}

/**
 * 取消当前流式输出（关闭 WebSocket 连接）
 */
export function cancelStream(): void {
  if (activeSocket) {
    const ws = activeSocket
    activeSocket = null
    ws.close()
  }
}

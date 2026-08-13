/**
 * 对话服务 - 会话管理、消息 API 与标准 SSE 流式对话
 * 对接后端 /api/v1/chat/sessions 与 POST /api/v1/chat/sse/:session_id
 *
 * SSE 协议要点：
 * - 每次发送消息发起一次 fetch POST，以 ReadableStream 消费 text/event-stream
 * - 事件：chunk（内容片段）/ done（完成，已入库）/ error（可读中文描述）
 * - 停止生成：AbortController.abort() 中断连接，无 stopped 确认事件，
 *   服务端将已生成内容（非空时）保存为 assistant 消息
 * - 心跳为 SSE 注释行（: ping），解析器忽略
 * - 请求级生命周期，无长驻连接，无需重连
 */
import { get, post, buildApiUrl, authHeaders } from './api'
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
export function getMessages(sessionId: string, params: ListParams = {}): Promise<PagedList<ChatMessage>> {
  return get<PagedList<ChatMessage>>(`/chat/sessions/${sessionId}/messages`, {
    page: params.page ?? 1,
    page_size: params.page_size ?? 200,
  })
}

/** 流式对话回调 */
export interface StreamHandlers {
  /** 收到内容片段 */
  onChunk: (content: string) => void
  /** 流式输出完成（完整回复已入库） */
  onDone: () => void
  /** 已被本地停止（abort；此前内容服务端已保存） */
  onStopped: () => void
  /** 服务端错误（可读中文描述），需恢复输入状态 */
  onError: (error: string) => void
  /** 流式生成中网络中断（已收到部分内容，该条回复可能不完整） */
  onInterrupted: () => void
}

// 当前活跃的流式请求（同一时间只允许一条）
let activeController: AbortController | null = null

/**
 * 停止当前流式生成（中断 HTTP 连接）
 * 服务端请求 context 取消 → 上游 LLM 流中断，部分内容入库
 */
export function stopStream(): void {
  activeController?.abort()
}

/**
 * 发送消息并以标准 SSE 流式接收回复
 * 同一时间只允许一条流式请求（重复调用会先中断上一条）
 */
export async function streamChatMessage(
  sessionId: string,
  content: string,
  handlers: StreamHandlers
): Promise<void> {
  stopStream()
  const controller = new AbortController()
  activeController = controller

  /** 是否已收到任何内容片段 */
  let received = false
  /** 是否已收到终止事件（done/error） */
  let finished = false

  try {
    const response = await fetch(buildApiUrl(`/chat/sse/${sessionId}`), {
      method: 'POST',
      headers: {
        'Accept': 'text/event-stream',
        'Content-Type': 'application/json',
        ...authHeaders(),
      },
      body: JSON.stringify({ content }),
      signal: controller.signal,
    })

    if (!response.ok || !response.body) {
      const errorData = await response.json().catch(() => ({}))
      handlers.onError(errorData.message || `请求失败（HTTP ${response.status}）`)
      return
    }

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let eventType = ''
    let dataLines: string[] = []

    /** 空行触发事件分发 */
    const dispatchEvent = () => {
      const type = eventType
      const data = dataLines.join('\n')
      eventType = ''
      dataLines = []
      if (!type) return

      let payload: { content?: string } = {}
      try {
        payload = data ? JSON.parse(data) : {}
      } catch {
        // 忽略无法解析的 data
      }

      if (type === 'chunk' && payload.content) {
        received = true
        handlers.onChunk(payload.content)
      } else if (type === 'done') {
        finished = true
        handlers.onDone()
      } else if (type === 'error') {
        finished = true
        handlers.onError(payload.content || '论道出错了')
      }
    }

    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      // 按行解析 SSE 帧（跨 chunk 行缓冲拼接）
      let idx: number
      while ((idx = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, idx).replace(/\r$/, '')
        buffer = buffer.slice(idx + 1)
        if (line === '') {
          dispatchEvent()
        } else if (line.startsWith(':')) {
          // 注释行（心跳），忽略
        } else if (line.startsWith('event:')) {
          eventType = line.slice('event:'.length).trim()
        } else if (line.startsWith('data:')) {
          dataLines.push(line.slice('data:'.length).trimStart())
        }
      }
      if (finished) break
    }

    // 流结束但未收到终止事件：连接中断
    if (!finished) {
      if (received) {
        handlers.onInterrupted()
      } else {
        handlers.onError('连接中断，请重试')
      }
    }
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      // 本地停止：服务端已保存部分内容，前端落定"已停止"
      handlers.onStopped()
      return
    }
    if (received) {
      handlers.onInterrupted()
    } else {
      handlers.onError(error instanceof Error ? error.message : '网络请求失败，请检查网络连接')
    }
  } finally {
    if (activeController === controller) {
      activeController = null
    }
  }
}

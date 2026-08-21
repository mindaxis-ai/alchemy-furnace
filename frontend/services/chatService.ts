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
import { get, post, put, del, buildApiUrl, authHeaders } from './api'
import type { ChatSession, ChatMessage, GroupMember, CreateSessionRequest, PagedList, ListParams } from './types'

/**
 * 获取会话列表
 */
export function listSessions(params: ListParams = {}): Promise<PagedList<ChatSession>> {
  return get<PagedList<ChatSession>>('/chat/sessions', {
    page: params.page ?? 1,
    page_size: params.page_size ?? 100,
  })
}

/** 按 UUID 直接获取会话元数据（深链不依赖会话列表分页）。 */
export function getSession(sessionId: string): Promise<ChatSession> {
  return get<ChatSession>(`/chat/sessions/${sessionId}`)
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

/**
 * 建群(≥2 位道人;首问答自动命名,无 title 入参)
 */
export function createGroupSession(memberAgentIds: string[]): Promise<ChatSession> {
  return post<ChatSession>('/chat/sessions', { type: 'group', member_agent_ids: memberAgentIds })
}

/**
 * 重命名会话
 */
export function renameSession(sessionId: string, title: string): Promise<ChatSession> {
  return put<ChatSession>(`/chat/sessions/${sessionId}`, { title })
}

/**
 * 邀请入群(已在群静默跳过)
 */
export function addMembers(sessionId: string, agentIds: string[]): Promise<{ members: GroupMember[] }> {
  return post<{ members: GroupMember[] }>(`/chat/sessions/${sessionId}/members`, { agent_ids: agentIds })
}

/**
 * 移出群成员
 */
export function removeMember(sessionId: string, agentId: string): Promise<{ members: GroupMember[] }> {
  return del<{ members: GroupMember[] }>(`/chat/sessions/${sessionId}/members/${agentId}`)
}

export interface StreamSpeakerInfo {
  agent_id: string
  agent_name: string
  agent_avatar?: string
}

export interface StreamChunk extends Partial<StreamSpeakerInfo> {
  content: string
}

export interface StreamErrorInfo extends Partial<StreamSpeakerInfo> {
  error_code?: string
  /** false=群成员本轮失败但流继续；true=整个请求终止。 */
  terminal: boolean
}

export interface StreamOptions {
  /** 重试最近一次同内容用户消息；服务端不得重复保存用户行。 */
  retry?: boolean
}

/** 流式对话回调 */
export interface StreamHandlers {
  /** 收到内容片段 */
  onChunk: (chunk: StreamChunk) => void
  /** 流式输出完成（完整回复已入库） */
  onDone: () => void
  /** 已被本地停止（abort；此前内容服务端已保存） */
  onStopped: () => void
  /** 服务端错误（可读中文描述），需恢复输入状态 */
  onError: (error: string, info: StreamErrorInfo) => void
  /** 流式生成中网络中断（已收到部分内容，该条回复可能不完整） */
  onInterrupted: () => void
  /** 群聊: 某道人开始发言(气泡身份头) */
  onSpeakerStart?: (info: StreamSpeakerInfo) => void
  /** 群聊: 某道人发言完毕(已入库) */
  onSpeakerDone?: (info: StreamSpeakerInfo & { message_id?: string; mentions?: ChatMessage['mentions'] }) => void
  /** 群聊: 回合结束(spoke=本回合发言总数) */
  onTurnDone?: (info: { spoke: number }) => void
  /** 自动命名成功(单/群聊) */
  onTitle?: (title: string) => void
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
  handlers: StreamHandlers,
  options: StreamOptions = {},
): Promise<void> {
  stopStream()
  const controller = new AbortController()
  activeController = controller

  /** terminal error / done / turn_done / stopped / transport interruption 恰好交付一次。 */
  let terminalDelivered = false
  const terminate = (callback: () => void) => {
    if (terminalDelivered) return
    terminalDelivered = true
    callback()
  }

  try {
    const response = await fetch(buildApiUrl(`/chat/sse/${sessionId}`), {
      method: 'POST',
      headers: {
        'Accept': 'text/event-stream',
        'Content-Type': 'application/json',
        ...authHeaders(),
      },
      body: JSON.stringify(options.retry ? { content, retry: true } : { content }),
      signal: controller.signal,
    })

    if (!response.ok || !response.body) {
      const errorData = await response.json().catch(() => ({}))
      terminate(() => handlers.onError(errorData.message || `请求失败（HTTP ${response.status}）`, { terminal: true }))
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

      let payload: Record<string, unknown> = {}
      try {
        payload = data ? JSON.parse(data) : {}
      } catch {
        // 忽略无法解析的 data
      }

      if (terminalDelivered) return
      if (type === 'chunk' && typeof payload.content === 'string') {
        handlers.onChunk(payload as unknown as StreamChunk)
      } else if (type === 'done') {
        terminate(handlers.onDone)
      } else if (type === 'error') {
        const terminal = payload.terminal !== false
        const info = { ...payload, terminal } as unknown as StreamErrorInfo
        const report = () => handlers.onError(typeof payload.content === 'string' ? payload.content : '论道出错了', info)
        if (terminal) terminate(report)
        else report()
      } else if (type === 'speaker_start') {
        handlers.onSpeakerStart?.(payload as unknown as StreamSpeakerInfo)
      } else if (type === 'speaker_done') {
        handlers.onSpeakerDone?.(payload as unknown as StreamSpeakerInfo & { message_id?: string; mentions?: ChatMessage['mentions'] })
      } else if (type === 'turn_done') {
        terminate(() => handlers.onTurnDone?.(payload as unknown as { spoke: number }))
      } else if (type === 'title' && typeof payload.title === 'string') {
        // 单/群聊 title 事件统一 {"title": "..."} 形态
        handlers.onTitle?.(payload.title)
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
      if (terminalDelivered) break
    }

    // 流结束但未收到终止事件：连接中断
    terminate(handlers.onInterrupted)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      // 本地停止：服务端已保存部分内容，前端落定"已停止"
      terminate(handlers.onStopped)
      return
    }
    terminate(handlers.onInterrupted)
  } finally {
    if (activeController === controller) {
      activeController = null
    }
  }
}

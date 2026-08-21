import type { StreamChunk, StreamSpeakerInfo } from '@/services/chatService'

/**
 * 标准 SSE 事件调度器。
 *
 * 服务端已经按 token 推送内容，前端只需保持事件原序交付。这里不再二次拆字、
 * 随机停顿或按定时器排队，避免快模型产生数十秒本地积压，也避免回合结束后
 * 仍显示“加载中”。React 会自动批处理同一帧内的状态更新。
 */

export type SpeakerStartInfo = StreamSpeakerInfo

export interface SpeakerDoneInfo {
  agent_id: string
  agent_name: string
  agent_avatar?: string
  message_id?: string
  mentions?: unknown
}

export interface DispatcherActions {
  onChunk: (chunk: StreamChunk) => void
  onSpeakerStart: (info: SpeakerStartInfo) => void
  onSpeakerDone: (info: SpeakerDoneInfo) => void
  onNotice: (text: string, isError: boolean, retryable?: boolean) => void
  onDrained: () => void
}

/** 保留类型以兼容旧验证脚本；标准流式交付不再使用人工节奏参数。 */
export interface DispatcherPace {
  intervalMs?: number
  speakerIntervalRange?: [number, number]
  speakerPauseRange?: [number, number]
  splitRunes?: number
  random?: () => number
}

export function createStreamDispatcher(actions: DispatcherActions, _pace?: DispatcherPace) {
  let ended = false
  let drained = false

  const finish = () => {
    if (ended && !drained) {
      drained = true
      actions.onDrained()
    }
  }

  return {
    pushChunk(chunk: StreamChunk) {
      if (!ended && chunk.content) actions.onChunk(chunk)
    },
    pushSpeakerStart(info: SpeakerStartInfo) {
      if (!ended) actions.onSpeakerStart(info)
    },
    pushSpeakerDone(info: SpeakerDoneInfo) {
      if (!ended) actions.onSpeakerDone(info)
    },
    pushNotice(text: string, isError: boolean, retryable?: boolean) {
      if (!ended) actions.onNotice(text, isError, retryable)
    },
    markDone() {
      ended = true
      finish()
    },
    /** 标准交付无本地积压；保留停止路径 API。 */
    flushNow() {},
  }
}

export type StreamDispatcher = ReturnType<typeof createStreamDispatcher>

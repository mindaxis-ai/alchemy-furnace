/**
 * 流式事件调度器 — 单聊与群聊共用的 typing 节奏控制
 *
 * 群聊"一次性加载"根因修复:
 *   旧实现(ChatContext 内嵌 createChunkDispatcher)在 speaker_start/speaker_done
 *   到达时 flushNow() 清空 typing 队列 — 网络远快于 30ms/片 的打字节奏,
 *   队列里积压的几十片被同步倒出,每个道人的气泡"瞬满",整回合看似一次性加载。
 *
 * 新设计: 控制标记(发言人开始/结束/通知)与内容 chunk 走同一条队列,
 *   以固定节拍按到达顺序依次消费 —— 切换发言人不再 flush,节奏贯穿整个回合,
 *   气泡一个接一个"打字"出来。归属正确性由队列顺序天然保证:
 *   SSE 事件本就有序,N+1 的 chunk 必定排在其 start 标记之后,不会落进上一条气泡。
 *
 * 节奏打磨(模拟多人聊天"有快有慢"):
 *   - 每位发言人开腔前随机停顿(speakerPauseRange,打字中的呼吸感)
 *   - 每位发言人随机定档自己的语速(speakerIntervalRange)
 *   - 超大 chunk(沉默探测缓冲首爆/provider L chunk)切成 ≤splitRunes 小片,抹平突喷
 *
 * flushNow 仅用于停止/中断/致命错误路径: 按序立即应用队列残余(标记同样生效),
 * 不触发 onDrained — 收尾由调用方自己的 finalize 动作(STOP_STREAM 等)完成,
 * 避免抢先 finalize 导致「已停止」标记找不到 id=-1 的临时消息。
 */

export interface SpeakerStartInfo {
  agent_id: string
  agent_name: string
  agent_avatar?: string
}

export interface SpeakerDoneInfo {
  agent_id: string
  message_id?: string
  /** 服务端落库的 mentions(ChatMessage.mentions 形态,调用方自行断言类型) */
  mentions?: unknown
}

export interface DispatcherActions {
  onChunk: (text: string) => void
  onSpeakerStart: (info: SpeakerStartInfo) => void
  onSpeakerDone: (info: SpeakerDoneInfo) => void
  /** 回合内通知(如群聊单道人失败): 按序插入系统条,不打断回合 */
  onNotice: (text: string, isError: boolean) => void
  /** 队列排空且服务端已结束(markDone 后),可 finalize 临时消息 */
  onDrained: () => void
}

export interface DispatcherPace {
  /** 单聊/默认节奏(每片间隔 ms),默认 30 ≈ DeepSeek 网页体感 */
  intervalMs?: number
  /** 群聊发言人语速区间(ms/片),默认 [18, 55] — 开腔时随机定档 */
  speakerIntervalRange?: [number, number]
  /** 发言人开腔前停顿区间(ms),默认 [400, 1200] */
  speakerPauseRange?: [number, number]
  /** 超大 chunk 切分粒度(rune),默认 4 */
  splitRunes?: number
  /** 随机源(测试注入确定性值) */
  random?: () => number
}

type Item =
  | { kind: 'chunk'; text: string }
  | { kind: 'start'; info: SpeakerStartInfo }
  | { kind: 'done'; info: SpeakerDoneInfo }
  | { kind: 'notice'; text: string; isError: boolean }

export function createStreamDispatcher(actions: DispatcherActions, pace: DispatcherPace = {}) {
  const rand = pace.random ?? Math.random
  const baseInterval = pace.intervalMs ?? 30
  const [siLo, siHi] = pace.speakerIntervalRange ?? [18, 55]
  const [spLo, spHi] = pace.speakerPauseRange ?? [400, 1200]
  const splitRunes = pace.splitRunes ?? 4

  const queue: Item[] = []
  let timer: ReturnType<typeof setTimeout> | null = null
  /** 服务端已结束(done/turn_done): 队列排空后触发 onDrained */
  let serverDone = false
  /** 当前打字间隔: 单聊恒为 baseInterval;发言人开腔时随机定档 */
  let interval = baseInterval

  const apply = (it: Item) => {
    switch (it.kind) {
      case 'chunk':
        actions.onChunk(it.text)
        return
      case 'start':
        actions.onSpeakerStart(it.info)
        return
      case 'done':
        actions.onSpeakerDone(it.info)
        return
      case 'notice':
        actions.onNotice(it.text, it.isError)
        return
    }
  }

  const tick = () => {
    const it = queue.shift()
    if (!it) {
      timer = null
      if (serverDone) {
        actions.onDrained()
      }
      return
    }
    apply(it)
    if (it.kind === 'start') {
      // 新发言人: 定档自己的语速 + 开腔前停顿("对方正在输入"的呼吸感)
      interval = Math.round(siLo + rand() * (siHi - siLo))
      timer = setTimeout(tick, Math.round(spLo + rand() * (spHi - spLo)))
      return
    }
    timer = setTimeout(tick, interval)
  }

  const kick = () => {
    if (timer === null) {
      // 首片立刻消费(不延迟首字),tick 内会调度下一拍
      tick()
    }
  }

  return {
    /** 内容片段入队(超大 chunk 按 splitRunes 切片,抹平突喷) */
    pushChunk(text: string) {
      const runes = Array.from(text)
      if (runes.length <= splitRunes) {
        queue.push({ kind: 'chunk', text })
      } else {
        for (let i = 0; i < runes.length; i += splitRunes) {
          queue.push({ kind: 'chunk', text: runes.slice(i, i + splitRunes).join('') })
        }
      }
      kick()
    },
    /** 发言人开腔标记入队(轮到它时才开气泡,此前队列按节奏播上一位) */
    pushSpeakerStart(info: SpeakerStartInfo) {
      queue.push({ kind: 'start', info })
      kick()
    },
    /** 发言人收尾标记入队(轮到它时才 finalize 该气泡,可用真实 message_id) */
    pushSpeakerDone(info: SpeakerDoneInfo) {
      queue.push({ kind: 'done', info })
      kick()
    },
    /** 回合内通知入队(群聊单道人失败等,不打断回合) */
    pushNotice(text: string, isError: boolean) {
      queue.push({ kind: 'notice', text, isError })
      kick()
    },
    /** 服务端已结束: 队列按节奏自然排空后触发 onDrained */
    markDone() {
      serverDone = true
      if (timer === null && queue.length === 0) {
        actions.onDrained()
      }
    },
    /**
     * 停止/中断/致命错误: 按序立即应用队列残余(chunk 与标记都不丢)
     * 不触发 onDrained — 收尾动作(STOP_STREAM 等)由调用方紧接着 dispatch,
     * 确保它还能看到 id=-1 的临时消息并打上「已停止」等标记。
     */
    flushNow() {
      if (timer !== null) {
        clearTimeout(timer)
        timer = null
      }
      let it: Item | undefined
      while ((it = queue.shift()) !== undefined) {
        apply(it)
      }
    },
  }
}

export type StreamDispatcher = ReturnType<typeof createStreamDispatcher>

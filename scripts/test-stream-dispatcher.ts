/**
 * streamDispatcher 一次性验证脚本(systematic-debugging Phase 4 失败测试)
 * 运行: node --experimental-strip-types frontend/scripts/test-stream-dispatcher.ts
 *
 * 编码的回归场景(群聊"一次性加载" bug):
 *   网络远快于 typing 节奏时,speaker_start/done 到达瞬间
 *   旧实现 flushNow() 把队列残余全部同步倒出 → 气泡"瞬满"。
 *   新实现必须把控制标记与 chunk 同队列按序消费,全程不断节奏。
 */
import { createStreamDispatcher } from '../frontend/services/streamDispatcher.ts'

type Applied =
  | { kind: 'chunk'; text: string }
  | { kind: 'start'; id: string }
  | { kind: 'done'; id: string }
  | { kind: 'notice'; text: string; isError: boolean }
  | { kind: 'drained' }

function harness() {
  const applied: Applied[] = []
  const dispatcher = createStreamDispatcher(
    {
      onChunk: (text) => applied.push({ kind: 'chunk', text }),
      onSpeakerStart: (info) => applied.push({ kind: 'start', id: info.agent_id }),
      onSpeakerDone: (info) => applied.push({ kind: 'done', id: info.agent_id }),
      onNotice: (text, isError) => applied.push({ kind: 'notice', text, isError }),
      onDrained: () => applied.push({ kind: 'drained' }),
    },
    // 确定性节奏: 1ms 一切,随机钉死 0.5,便于断言
    { intervalMs: 1, speakerIntervalRange: [1, 1], speakerPauseRange: [1, 1], splitRunes: 4, random: () => 0.5 },
  )
  return { applied, dispatcher }
}

function waitFor(applied: Applied[], pred: (a: Applied[]) => boolean, ms = 3000): Promise<void> {
  return new Promise((resolve, reject) => {
    const t0 = Date.now()
    const i = setInterval(() => {
      if (pred(applied)) { clearInterval(i); resolve() }
      else if (Date.now() - t0 > ms) { clearInterval(i); reject(new Error('等待超时: ' + JSON.stringify(applied.slice(0, 6)))) }
    }, 1)
  })
}

let failures = 0
function check(name: string, cond: boolean, detail = '') {
  if (cond) { console.log(`  ✅ ${name}`) } else { failures++; console.error(`  ❌ ${name} ${detail}`) }
}

async function main() {
  // ─── 用例 1(回归): 同步 burst 灌入整个回合,不得同步倒出 ───
  console.log('用例1: 群聊 burst — 发言人切换不再瞬倒')
  {
    const { applied, dispatcher } = harness()
    // 模拟快网络: 整个回合事件在一个 JS 同步块内到达
    dispatcher.pushSpeakerStart({ agent_id: 'A', agent_name: '甲' })
    for (let i = 0; i < 20; i++) dispatcher.pushChunk(`a${i} `)
    dispatcher.pushSpeakerDone({ agent_id: 'A', message_id: 'm1' })
    dispatcher.pushSpeakerStart({ agent_id: 'B', agent_name: '乙' })
    for (let i = 0; i < 20; i++) dispatcher.pushChunk(`b${i} `)
    dispatcher.pushSpeakerDone({ agent_id: 'B', message_id: 'm2' })
    dispatcher.markDone()

    // 同步块结束瞬间: 只允许消费了极少量(开场),绝不能 40+ 片全倒出
    check('burst 后仅消费开头少量', applied.length <= 4, `实际 ${applied.length}`)
    check('首个动作是 A 开气泡', applied[0]?.kind === 'start' && (applied[0] as any).id === 'A')

    await waitFor(applied, (a) => a.some((x) => x.kind === 'drained'))

    // 全序断言: start A → chunks A → done A → start B → chunks B → done B → drained
    const kinds = applied.map((x) => x.kind)
    const iStartA = kinds.indexOf('start')
    const iDoneA = kinds.indexOf('done')
    const iStartB = kinds.indexOf('start', iDoneA)
    const iDoneB = kinds.indexOf('done', iStartB)
    const iDrained = kinds.indexOf('drained')
    check('顺序: startA < doneA < startB < doneB < drained',
      iStartA >= 0 && iStartA < iDoneA && iDoneA < iStartB && iStartB < iDoneB && iDoneB < iDrained)
    const chunksA = applied.slice(iStartA, iDoneA).filter((x) => x.kind === 'chunk')
    const chunksB = applied.slice(iStartB, iDoneB).filter((x) => x.kind === 'chunk')
    check('A 的 chunk 全部在 startA~doneA 之间', chunksA.length > 0 && chunksA.every((c) => (c as any).text.startsWith('a')))
    check('B 的 chunk 全部在 startB~doneB 之间', chunksB.length > 0 && chunksB.every((c) => (c as any).text.startsWith('b')))
    check('drained 仅一次', kinds.filter((k) => k === 'drained').length === 1)
    // 内容完整性(拼接还原)
    const textA = chunksA.map((c) => (c as any).text).join('')
    check('A 内容拼接无损', textA === Array.from({ length: 20 }, (_, i) => `a${i} `).join(''))
  }

  // ─── 用例 2: 单聊 markDone 自然排空 ───
  console.log('用例2: 单聊 — markDone 后排空才 drained')
  {
    const { applied, dispatcher } = harness()
    dispatcher.pushChunk('你好')
    dispatcher.pushChunk('，世界')
    dispatcher.markDone()
    check('同步段只消费首片', applied.length === 1)
    await waitFor(applied, (a) => a.some((x) => x.kind === 'drained'))
    check('chunk 全部按序', JSON.stringify(applied.slice(0, -1).map((x) => (x as any).text)) === '["你好","，世界"]')
    check('drained 收尾', applied[applied.length - 1].kind === 'drained')
  }

  // ─── 用例 3: flushNow(停止路径)按序立即应用,不触发 drained ───
  console.log('用例3: flushNow — 按序清仓,标记同样生效')
  {
    const { applied, dispatcher } = harness()
    dispatcher.pushSpeakerStart({ agent_id: 'A', agent_name: '甲' })
    for (let i = 0; i < 10; i++) dispatcher.pushChunk(`x${i}`)
    dispatcher.flushNow()
    const kinds = applied.map((x) => x.kind)
    check('flush 后立刻全部应用', kinds[0] === 'start' && kinds.filter((k) => k === 'chunk').length >= 3)
    check('flush 不触发 drained(由调用方自己的 finalize 动作收尾)', !kinds.includes('drained'))
    // flush 后再推新片仍可工作(回合未死)
    dispatcher.pushChunk('tail')
    await waitFor(applied, (a) => (a[a.length - 1] as any)?.text === 'tail')
    check('flush 后仍可继续消费', true)
  }

  // ─── 用例 4: 超大 chunk 切分(探测缓冲首爆/L chunk 抹平) ───
  console.log('用例4: 大 chunk 切分且拼接无损')
  {
    const { applied, dispatcher } = harness()
    dispatcher.pushChunk('一'.repeat(30))
    dispatcher.markDone()
    await waitFor(applied, (a) => a.some((x) => x.kind === 'drained'))
    const chunks = applied.filter((x) => x.kind === 'chunk')
    check('被切成多片', chunks.length >= 8, `实际 ${chunks.length} 片`)
    check('每片 ≤4 字', chunks.every((c) => Array.from((c as any).text).length <= 4))
    check('拼接无损', chunks.map((c) => (c as any).text).join('') === '一'.repeat(30))
  }

  // ─── 用例 5: 群聊单道人失败 → notice 按序插入,回合继续 ───
  console.log('用例5: notice 按序落位')
  {
    const { applied, dispatcher } = harness()
    dispatcher.pushSpeakerStart({ agent_id: 'A', agent_name: '甲' })
    dispatcher.pushChunk('话说一半')
    dispatcher.pushNotice('乙的凭证缺失', true)
    dispatcher.pushSpeakerStart({ agent_id: 'C', agent_name: '丙' })
    dispatcher.pushChunk('丙接上')
    dispatcher.markDone()
    await waitFor(applied, (a) => a.some((x) => x.kind === 'drained'))
    const kinds = applied.map((x) => x.kind)
    const iNotice = kinds.indexOf('notice')
    check('notice 在 A chunk 之后、C start 之前',
      iNotice > kinds.indexOf('start') && iNotice < kinds.indexOf('start', iNotice))
    check('notice 携带错误标记', (applied[iNotice] as any).isError === true)
  }

  console.log(failures === 0 ? '\n全部通过 ✅' : `\n${failures} 项失败 ❌`)
  process.exit(failures === 0 ? 0 : 1)
}

main()

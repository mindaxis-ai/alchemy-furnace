/** 标准 SSE 调度器回归验证：同步、原序、单次收尾。 */
import { createStreamDispatcher } from '../frontend/services/streamDispatcher.ts'

type Applied = { kind: string; value?: string }
const applied: Applied[] = []
const dispatcher = createStreamDispatcher({
  onChunk: chunk => applied.push({ kind: 'chunk', value: chunk.content }),
  onSpeakerStart: info => applied.push({ kind: 'start', value: info.agent_id }),
  onSpeakerDone: info => applied.push({ kind: 'done', value: info.agent_id }),
  onNotice: value => applied.push({ kind: 'notice', value }),
  onDrained: () => applied.push({ kind: 'drained' }),
})

dispatcher.pushSpeakerStart({ agent_id: 'A', agent_name: '甲' })
dispatcher.pushChunk({ content: '第一片' })
dispatcher.pushChunk({ content: '第二片' })
dispatcher.pushSpeakerDone({ agent_id: 'A', message_id: 'm1' })
dispatcher.pushSpeakerStart({ agent_id: 'B', agent_name: '乙' })
dispatcher.pushChunk({ content: '第三片' })
dispatcher.pushSpeakerDone({ agent_id: 'B', message_id: 'm2' })
dispatcher.markDone()
dispatcher.markDone()

const actual = applied.map(item => `${item.kind}:${item.value || ''}`)
const expected = [
  'start:A',
  'chunk:第一片',
  'chunk:第二片',
  'done:A',
  'start:B',
  'chunk:第三片',
  'done:B',
  'drained:',
]

if (JSON.stringify(actual) !== JSON.stringify(expected)) {
  console.error('标准 SSE 事件顺序不正确', { actual, expected })
  process.exit(1)
}

console.log('标准 SSE 调度器验证通过')

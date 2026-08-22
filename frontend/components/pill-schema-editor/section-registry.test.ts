import { describe, expect, it } from 'vitest'

import {
  draftToSchema,
  schemaToDraft,
} from '@/components/pill-schema-editor/section-registry'
import type { SkillSchema } from '@/services/types'

const original: SkillSchema = {
  identity_card: '炼丹师',
  expression_dna: { formality: 0.2, vocabulary: ['炉'] },
  values: ['克己'],
  fusion_lineage: {
    parents: [{ uuid: 'p-1', name: '父丹' }],
    operator: { id: 'op-1', name: '随机融合' },
    fused_at: '2026-08-20T00:00:00Z',
  },
  future_unknown: { nested: ['甲'] },
}

describe('pill-schema-editor section registry', () => {
  it('schemaToDraft 对缺失/类型不符的区块给空兜底', () => {
    // 服务端落库数据可能带历史脏类型,读取端必须宽容(此处故意外层断言绕过静态类型)
    const messy = { identity_card: 'x', values: '不是数组' } as unknown as SkillSchema
    const draft = schemaToDraft(messy)
    expect(draft.identity_card).toBe('x')
    expect(draft.expression_dna).toEqual({})
    expect(draft.mental_models).toEqual([])
    expect(draft.decision_heuristics).toEqual([])
    expect(draft.values).toEqual([])
    expect(draft.anti_patterns).toEqual([])
    expect(draft.honest_limits).toEqual([])
    expect(draft.example_dialogues).toEqual([])
  })

  it('schemaToDraft 深复制，改草稿不污染源 schema', () => {
    const src: SkillSchema = { mental_models: [{ name: '阴阳' }], values: ['克己'] }
    const draft = schemaToDraft(src)
    draft.mental_models.push({ name: '新增' })
    draft.values.push('慎独')
    expect(src.mental_models).toHaveLength(1)
    expect(src.values).toEqual(['克己'])
  })

  it('draftToSchema 以原始 schema 为底合并：已知键被草稿覆盖，未知键原样保留', () => {
    const draft = schemaToDraft(original)
    draft.identity_card = '改'
    draft.values = ['克己', '慎独']
    const merged = draftToSchema(original, draft)

    expect(merged.identity_card).toBe('改')
    expect(merged.values).toEqual(['克己', '慎独'])
    // 未知键(融合血统 + 未来字段)原样保留:内容不变且引用不重建
    expect(merged.fusion_lineage).toBe(original.fusion_lineage)
    expect(merged.future_unknown).toBe(original.future_unknown)
    // 已知键中未被草稿触碰的字段保持原值
    expect(merged.expression_dna).toEqual({ formality: 0.2, vocabulary: ['炉'] })
  })

  it('原本缺失且草稿为空的已知键不新增；原本存在的键允许被清空', () => {
    const draft = schemaToDraft(original)
    const merged = draftToSchema(original, draft)
    // original 没有这些键,草稿空值不得新增
    expect('anti_patterns' in merged).toBe(false)
    expect('honest_limits' in merged).toBe(false)
    expect('mental_models' in merged).toBe(false)
    // original 有 values,草稿清空后必须显式写入空数组(用户主动清空)
    draft.values = []
    const cleared = draftToSchema(original, draft)
    expect(cleared.values).toEqual([])
  })
})

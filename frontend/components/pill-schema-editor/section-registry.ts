/**
 * 金丹 schema 编辑器区块注册表
 * 只负责已知字段的读写；序列化以原始 skill_schema 为底合并已知字段，未知键原样保留（开闭原则）
 * 新增已知区块 = 在 sectionCodecs 追加一条，无需改动序列化核心
 */
import type {
  ExpressionDNA,
  MentalModel,
  DecisionHeuristic,
  ExampleDialogue,
  PillSchemaDraft,
  SkillSchema,
} from '@/services/types'

/** 单个已知区块的编解码器 */
interface SectionCodec<T> {
  /** 草稿空值（缺失区块兜底 + 「空则不新增」判定基准） */
  readonly empty: T
  /** 宽容读取：类型不符返回 empty；数组/对象深拷贝隔离源 schema */
  read(schema: SkillSchema): T
  /** 判断草稿值是否为空（决定原本缺失的键是否写入） */
  isEmpty(value: T): boolean
}

/** schema 内容均为 JSON 兼容结构，JSON 往返即深拷贝 */
function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function readArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? deepClone(value as T[]) : []
}

function readObject<T extends object>(value: unknown, empty: T): T {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (deepClone(value) as T)
    : empty
}

function isEmptyUnknown(value: unknown): boolean {
  if (value === '') return true
  if (Array.isArray(value)) return value.length === 0
  return value !== null && typeof value === 'object' && Object.keys(value).length === 0
}

/** 已知区块注册表（新增区块在此追加一条即可） */
const sectionCodecs: { [K in keyof PillSchemaDraft]: SectionCodec<PillSchemaDraft[K]> } = {
  identity_card: {
    empty: '',
    read: schema => readString(schema.identity_card),
    isEmpty: value => value === '',
  },
  expression_dna: {
    empty: {},
    read: schema => readObject<ExpressionDNA>(schema.expression_dna, {}),
    isEmpty: isEmptyUnknown,
  },
  mental_models: {
    empty: [],
    read: schema => readArray<MentalModel>(schema.mental_models),
    isEmpty: isEmptyUnknown,
  },
  decision_heuristics: {
    empty: [],
    read: schema => readArray<DecisionHeuristic>(schema.decision_heuristics),
    isEmpty: isEmptyUnknown,
  },
  values: {
    empty: [],
    read: schema => readArray<string>(schema.values),
    isEmpty: isEmptyUnknown,
  },
  anti_patterns: {
    empty: [],
    read: schema => readArray<string>(schema.anti_patterns),
    isEmpty: isEmptyUnknown,
  },
  honest_limits: {
    empty: [],
    read: schema => readArray<string>(schema.honest_limits),
    isEmpty: isEmptyUnknown,
  },
  example_dialogues: {
    empty: [],
    read: schema => readArray<ExampleDialogue>(schema.example_dialogues),
    isEmpty: isEmptyUnknown,
  },
}

/** 编辑器已知的全部区块键（顺序即详情页展示顺序） */
export const PILL_SCHEMA_SECTION_KEYS = Object.keys(sectionCodecs) as Array<keyof PillSchemaDraft>

/** 从服务端 schema 读取编辑器草稿：已知区块宽容读取并深拷贝，未知键不进入草稿 */
export function schemaToDraft(schema: SkillSchema): PillSchemaDraft {
  return {
    identity_card: sectionCodecs.identity_card.read(schema),
    expression_dna: sectionCodecs.expression_dna.read(schema),
    mental_models: sectionCodecs.mental_models.read(schema),
    decision_heuristics: sectionCodecs.decision_heuristics.read(schema),
    values: sectionCodecs.values.read(schema),
    anti_patterns: sectionCodecs.anti_patterns.read(schema),
    honest_limits: sectionCodecs.honest_limits.read(schema),
    example_dialogues: sectionCodecs.example_dialogues.read(schema),
  }
}

/**
 * 序列化草稿：以原始 skill_schema 为底合并已知字段
 * - 未知键（fusion_lineage、未来字段）原样保留，引用不重建
 * - 已知键原本存在 → 以草稿为准（允许用户显式清空）
 * - 已知键原本缺失且草稿为空 → 不新增，避免污染
 */
export function draftToSchema(original: SkillSchema, draft: PillSchemaDraft): SkillSchema {
  const merged: SkillSchema = { ...original }
  const writable = merged as Record<string, unknown>
  for (const key of PILL_SCHEMA_SECTION_KEYS) {
    const codec = sectionCodecs[key] as SectionCodec<unknown>
    const value: unknown = draft[key]
    const presentInOriginal = Object.prototype.hasOwnProperty.call(original, key)
    if (presentInOriginal || !codec.isEmpty(value)) {
      writable[key] = value
    }
  }
  return merged
}

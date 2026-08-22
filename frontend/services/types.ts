/**
 * 数据类型定义
 * 对应「金丹化性」后端 API 的前端 TypeScript 类型
 * 金丹 = nuwa-skill 结构化语言模式技能包（非知识库）
 */

// ========== 金丹 skill_schema（nuwa-skill 结构） ==========

/** 句式长度 */
export type SentenceLength = 'short' | 'medium' | 'long' | 'mixed'

/** 表达 DNA */
export interface ExpressionDNA {
  sentence_length?: SentenceLength
  /** 正式程度 0-1 */
  formality?: number
  /** 高频词 */
  vocabulary?: string[]
  /** 禁用词 */
  taboo_words?: string[]
  /** 节奏 */
  rhythm?: string
  /** 幽默类型 */
  humor_type?: string
  /** 确定性表达风格 */
  certainty_style?: string
  /** 引用习惯 */
  citation_habit?: string
}

/** 心智模型 */
export interface MentalModel {
  name: string
  one_liner?: string
  source_evidence?: string[]
  application?: string
  detection_questions?: string[]
  limitations?: string[]
}

/** 决策启发式 */
export interface DecisionHeuristic {
  condition: string
  action: string
  case?: string
}

/** 示例对话 */
export interface ExampleDialogue {
  user: string
  assistant: string
}

/** 金丹结构化技能包 */
export interface SkillSchema {
  /** 第一人称身份卡 */
  identity_card?: string
  expression_dna?: ExpressionDNA
  mental_models?: MentalModel[]
  decision_heuristics?: DecisionHeuristic[]
  /** 价值观 */
  values?: string[]
  /** 反模式 */
  anti_patterns?: string[]
  /** 诚实边界 */
  honest_limits?: string[]
  example_dialogues?: ExampleDialogue[]
  /** 融合血统：仅融合产生的金丹持有（006-pill-fusion） */
  fusion_lineage?: FusionLineage
  [key: string]: unknown
}

/**
 * 金丹编辑器草稿：仅含编辑器已知区块（pill-schema-editor/section-registry 负责读写）
 * 序列化时以原始 skill_schema 为底合并已知字段，未知键（fusion_lineage、未来字段）原样保留
 */
export interface PillSchemaDraft {
  identity_card: string
  expression_dna: ExpressionDNA
  mental_models: MentalModel[]
  decision_heuristics: DecisionHeuristic[]
  values: string[]
  anti_patterns: string[]
  honest_limits: string[]
  example_dialogues: ExampleDialogue[]
}

/** 女娲蒸馏产生的可编辑草稿（不会自动落库） */
export interface DistillationSource {
  title: string
  url: string
  dimension: string
}

export interface DistillationDraft {
  name: string
  description: string
  persona_summary: string
  tags: string[]
  skill_schema: SkillSchema
  sources: DistillationSource[]
  model: string
}

/** 融合血统：父代金丹 + 算子 + 时间 */
export interface FusionLineage {
  parents: Array<{ uuid: string; name: string }>
  operator: { id: string; name: string }
  fused_at: string
}

// ========== 金丹 ==========

/** 金丹（语言模式/人格特质技能包） */
export interface Pill {
  id: string
  name: string
  description?: string
  skill_schema: SkillSchema
  tags: string[]
  author?: string
  version: string
  is_builtin: boolean
  created_at: string
  updated_at: string
}

// ========== 道人 ==========

/** Agent 状态 */
export type AgentStatus = 'active' | 'inactive'

/** 道人（AI Agent） */
export interface Agent {
  id: string
  name: string
  avatar?: string
  personality?: string
  model_name: string
  status: AgentStatus
  /** 主动性/表达欲 0-100(群聊发言欲) */
  proactivity: number
  created_at: string
  updated_at?: string
}

/** 服用记录（Agent 绑定金丹） */
export interface AgentPill {
  id: string
  agent_id: string
  pill_id: string
  /** 剂量/权重 0-10 */
  weight: number
  /** 服用顺序 */
  sort_order: number
  created_at: string
  pill?: Pill
}

/** 丹性相冲严重程度 */
export type TensionSeverity = 'low' | 'medium' | 'high'

/** 内在冲突（丹性相冲）：多金丹风格冲突检测结果 */
export interface InnerTension {
  /** 冲突维度，如「句式长度」「正式程度」 */
  dimension: string
  /** 冲突描述 */
  description: string
  /** 严重程度 */
  severity: TensionSeverity
}

/** 语言模式缓存 */
export interface LanguagePattern {
  is_valid: boolean
  system_prompt: string
  emergence_rules: string[]
  inner_tensions: InnerTension[]
}

/** 道人详情（含服用记录与语言模式） */
export interface AgentDetail extends Agent {
  agent_pills?: AgentPill[]
  language_pattern?: LanguagePattern | null
}

// ========== 对话 ==========

/** 流终止后的显式恢复协议；none 为安全默认。 */
export type ChatRecoveryMode = 'none' | 'resend' | 'persisted_retry'

/** 对话会话 */
/** 群成员 */
export interface GroupMember {
  agent_id: string
  name: string
  avatar?: string
  proactivity: number
  status?: AgentStatus
}

export interface ChatSession {
  id: string
  /** 会话类型: single 1v1 / group 多道人群 */
  type?: 'single' | 'group'
  /** single: 所属道人 UUID;group 留空 */
  agent_id: string
  title?: string
  created_at: string
  updated_at: string
  agent?: Agent
  /** 会话加载响应中的当前道人状态，用于历史只读提示。 */
  agent_status?: AgentStatus
  /** group: 群成员列表(按发言顺序) */
  members?: GroupMember[]
}

/** 后端权威的可对话就绪状态(GET /chat/readiness);ready_agent_ids 是允许发起会话的唯一名单 */
export interface ChatReadiness {
  active_agent_count: number
  ready_agent_ids: string[]
  can_create_single: boolean
  can_create_group: boolean
}

/** 对话消息（无 RAG 引用来源） */
export interface ChatMessage {
  id: string
  /** 会话 ID（后端消息列表不输出，仅本地构造的临时消息携带） */
  session_id?: string
  role: 'user' | 'assistant' | 'system'
  content: string
  created_at: string
  /** 断线导致该回复可能不完整 */
  incomplete?: boolean
  /** 该回复被手动停止 */
  stopped?: boolean
  /** 服务端错误消息（以错误气泡展示） */
  is_error?: boolean
  /** 仅终止整个传输的错误/不完整回复允许重试；成员级错误不允许。 */
  retryable?: boolean
  /** resend=正常重发但复用本地用户气泡；persisted_retry=复用后端用户行。 */
  recovery?: ChatRecoveryMode
  /** 群聊: 发言道人 UUID */
  agent_id?: string
  /** 群聊: 发言道人名(气泡身份头) */
  agent_name?: string
  /** 群聊流式 speaker_start 携带的头像，确保临时气泡身份稳定。 */
  agent_avatar?: string
  /** @提及: agents=道人 UUID 数组;user=是否@了用户 */
  mentions?: { agents?: string[]; user?: boolean }
}

// ========== 请求 ==========

/** 创建金丹请求 */
export interface CreatePillRequest {
  name: string
  description?: string
  skill_schema: SkillSchema
  tags?: string[]
  author?: string
  version?: string
}

/** 更新金丹请求 */
export type UpdatePillRequest = Partial<CreatePillRequest>

/** 创建道人请求 */
export interface CreateAgentRequest {
  name: string
  avatar?: string
  personality?: string
  model_name?: string
  proactivity?: number
}

/** 更新道人请求 */
export interface UpdateAgentRequest extends Partial<CreateAgentRequest> {
  status?: AgentStatus
}

/** 服用金丹请求 */
export interface BindPillRequest {
  pill_id: string
  weight: number
  sort_order: number
}

/** 更新服用记录请求 */
export interface UpdateAgentPillRequest {
  weight: number
  sort_order: number
}

/** 创建会话请求 */
//   - single: agent_id 必填
//   - group: type="group" + member_agent_ids ≥2
//   - title 字段忽略(自动命名)
export interface CreateSessionRequest {
  agent_id?: string
  type?: 'single' | 'group'
  member_agent_ids?: string[]
  title?: string
}

// ========== 响应 ==========

/** 分页列表 */
export interface PagedList<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

/** 列表查询参数 */
export interface ListParams {
  page?: number
  page_size?: number
}

/** 金丹列表查询参数 */
export interface PillListParams extends ListParams {
  keyword?: string
  is_builtin?: boolean
}

/** 道人列表查询参数 */
export interface AgentListParams extends ListParams {
  status?: AgentStatus
}

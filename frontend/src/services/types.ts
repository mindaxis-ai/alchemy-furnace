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
  [key: string]: unknown
}

// ========== 金丹 ==========

/** 金丹（语言模式/人格特质技能包） */
export interface Pill {
  id: number
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
  id: number
  name: string
  avatar?: string
  personality?: string
  model_name: string
  status: AgentStatus
  created_at: string
  updated_at?: string
}

/** 服用记录（Agent 绑定金丹） */
export interface AgentPill {
  id: number
  agent_id: number
  pill_id: number
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

/** 对话会话 */
export interface ChatSession {
  id: number
  agent_id: number
  title?: string
  created_at: string
  updated_at: string
  agent?: Agent
}

/** 对话消息（无 RAG 引用来源） */
export interface ChatMessage {
  id: number
  session_id: number
  role: 'user' | 'assistant' | 'system'
  content: string
  created_at: string
}

// ========== 模型配置（前端静态可选列表） ==========

/** 模型配置 */
export interface ModelConfig {
  id: string
  name: string
  description: string
  provider: string
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
}

/** 更新道人请求 */
export interface UpdateAgentRequest extends Partial<CreateAgentRequest> {
  status?: AgentStatus
}

/** 服用金丹请求 */
export interface BindPillRequest {
  pill_id: number
  weight: number
  sort_order: number
}

/** 更新服用记录请求 */
export interface UpdateAgentPillRequest {
  weight: number
  sort_order: number
}

/** 创建会话请求 */
export interface CreateSessionRequest {
  agent_id: number
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

// ========== WebSocket ==========

/** WebSocket 服务端消息（chunk / done / error） */
export interface WSMessage {
  type: 'chunk' | 'done' | 'error'
  content?: string
}

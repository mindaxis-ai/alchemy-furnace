/**
 * 数据类型定义
 * 对应「金丹化性」后端 API 的前端 TypeScript 类型
 * 金丹 = nuwa-skill 结构化语言模式技能包（非知识库）
 */
import type { PillItemState } from '../lib/pill-inventory-state'

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

/** 蒸馏研究摘要(证据等级与来源统计,不含正文) */
export interface DistillationResearchSummary {
  evidence_level: 'limited' | 'standard'
  document_count: number
  domain_count: number
  total_characters: number
  warnings: string[]
}

export interface DistillationDraft {
  name: string
  description: string
  persona_summary: string
  avatar?: string
  tags: string[]
  skill_schema: SkillSchema
  sources: DistillationSource[]
  model: string
  research: DistillationResearchSummary
}

// ========== Skill 导出 ==========

/** 导出目标平台 */
export type ExportFormat = 'codex' | 'claude'

/** 导出来源引用(仅标题/URL/维度,不含网页正文) */
export interface ExportableSkillSource {
  title: string
  url: string
  dimension: string
}

/**
 * 可导出的规范化 Skill 模型(plan §2)。
 * 独立于数据库 Pill 的投影: 名称/slug/描述/指令/结构化技能包/来源/归属;
 * instructions 由结构化字段稳定渲染,slug 由服务端派生。
 */
export interface ExportableSkill {
  name: string
  slug: string
  description: string
  instructions: string
  skillSchema: SkillSchema
  tags: string[]
  sources: ExportableSkillSource[]
  attribution: { name: 'nuwa-skill'; license: 'MIT'; url: string }
  generatedAt: string
}

/**
 * Skill 导出请求: 已保存金丹的结构化数据(skill) / 丹方版本(recipe_id 为当前版本,
 * 带 revision_id 为指定版本) / 旧 pill ID(pill_id, 仅经 LegacyMap 解析) 三选一。
 * 接口绝不接收 API Key。
 */
export interface SkillExportRequest {
  pill_id?: string
  recipe_id?: string
  revision_id?: string
  skill?: ExportableSkill
  format: ExportFormat
}

/** Skill 导出结果: ZIP 字节 + 浏览器下载文件名 */
export interface SkillExportResult {
  blob: Blob
  filename: string
}

/** 融合血统：父代金丹 + 算子 + 时间 */
export interface FusionLineage {
  parents: Array<{ uuid: string; name: string }>
  operator: { id: string; name: string }
  fused_at: string
}

// ========== 金丹 ==========

/**
 * 金丹（语言模式/人格特质技能包）
 * Pill 当前无 avatar 数据契约（2026-08-28 产品边界，见计划）；金丹封面需另行迁移。
 * 金丹卡片保持 FlaskConical 丹瓶类型图标，前端不得读取不存在的字段。
 */
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
  /** 是否启用本地记忆(检索/蒸馏);旧服务端未返回该字段时按未启用处理 */
  memory_enabled?: boolean
  created_at: string
  updated_at?: string
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

/** 道人详情（含已吸收能力快照与语言模式缓存；能力编排以 pillInventoryService effects 为准） */
export interface AgentDetail extends Agent {
  language_pattern?: LanguagePattern | null
}

/** 道人编辑器草稿中的能力编排行（本地稳定 key 供受控列表渲染/排序） */
export interface AgentEffectDraftItem {
  key: string
  /** 能力 UUID（服用快照；与库存 itemId / 丹方 recipeId 严格区分） */
  effect_id: string
  /** 剂量/权重 0-10 */
  weight: number
}

/** 道人编辑器草稿：与服务器对象零引用共享的独立编辑副本 */
export interface AgentEditorDraft {
  name: string
  avatar: string
  personality: string
  model_name: string
  proactivity: number
  status: AgentStatus
  /** 能力编排（已吸收能力全量；提交集必须等于活跃集） */
  effects: AgentEffectDraftItem[]
}

// ========== 本地记忆 ==========

/** 记忆类型(spec §10.1) */
export type MemoryKind = 'user_fact' | 'user_preference' | 'relationship' | 'open_loop' | 'episode'

/** 记忆状态 */
export type MemoryStatus = 'active' | 'superseded' | 'archived'

/** 道人本地记忆(agent_memories) */
export interface AgentMemory {
  id: string
  kind: MemoryKind
  content: string
  keywords: string[]
  importance: number
  confidence: number
  pinned: boolean
  status: MemoryStatus
  /** 来源会话 UUID(无来源如手工录入时缺省) */
  source_session_id?: string
  /** 来源消息 UUID(无来源时缺省) */
  source_message_id?: string
  created_at: string
  updated_at: string
}

/** 创建记忆请求 */
export interface CreateMemoryRequest {
  kind: MemoryKind
  content: string
  keywords?: string[]
  importance?: number
  confidence?: number
  pinned?: boolean
}

/** 更新记忆请求(PATCH 语义:仅传变更字段) */
export type UpdateMemoryRequest = Partial<CreateMemoryRequest>

// ========== 对话 ==========

/** 流终止后的显式恢复协议；none 为安全默认。 */
export type ChatRecoveryMode = 'none' | 'resend' | 'persisted_retry'

/** 调试模式下仅存在于当前前端消息树中的实际模型输入快照。 */
export interface PromptDebugPayload {
  agent_id?: string
  agent_name?: string
  agent_avatar?: string
  model: string
  messages: Array<{ role: string; content: string }>
  /** LangGraph 运行期已无预算概念(legacy GenerationOptions 契约遗留),后端恒为 0 或缺失;UI 仅在非零时展示。 */
  generation?: {
    max_tokens: number
    max_sentences: number
  }
}

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
  /** single: 服务端预加载的真实道号;group 为空串 */
  agent_name?: string
  /** single: 服务端预加载的道人头像;group 为空串 */
  agent_avatar?: string
  /** group: 群聊自定义头像 */
  avatar?: string
  title?: string
  created_at: string
  updated_at: string
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
  /** 不由历史 API 持久化；刷新或切换会话后消失。 */
  prompt_debug?: PromptDebugPayload
  /** 编排回合标识(仅流式临时消息携带,不持久化):续跑控件据此定位 interrupted run */
  run_id?: string
}

// ========== 请求 ==========

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
  /** 是否启用本地记忆(检索/蒸馏) */
  memory_enabled?: boolean
}

/** 创建会话请求 */
//   - single: agent_id 必填
//   - group: type="group" + member_agent_ids ≥2
//   - title: single 忽略;group 接受可选主题(空值/空白自动命名)
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

/** 道人列表查询参数 */
export interface AgentListParams extends ListParams {
  status?: AgentStatus
}

// ========== 丹方与消耗品库存（金丹消耗品重构） ==========
// 契约对齐 backend/go/server/http/gateway/web/handler/pill_inventory 输出。
// 四类实体标识各有专属属性名：recipe_id / revision_id / item_id / effect_id，
// 禁止用一个含糊的 Pill.id 混用（任务 6 硬约束）。

/** 丹方列表项（UUID 由后端显式携带） */
export interface RecipeListItem {
  id: string
  name: string
  current_revision_id: string
  archived_at?: string | null
  created_at: string
  /** 该丹方当前可用金丹实例数（GROUP BY 聚合） */
  available_count: number
  /** 当前版本序号（任务 6 丹方入口显示「版本 vN」） */
  revision: number
}

/** 丹方详情（含当前版本内容；任意状态可读） */
export interface RecipeDetail {
  id: string
  name: string
  description: string
  skill_schema: SkillSchema
  tags: string[]
  author: string
  version_label: string
  revision: number
  current_revision_id: string
  archived_at?: string | null
  created_at: string
}

/** 丹方不可变版本（revision 从 1 递增；旧金丹不受新版本影响） */
export interface RecipeRevision {
  id: string
  revision: number
  name: string
  description: string
  skill_schema: SkillSchema
  tags: string[]
  author: string
  version_label: string
  created_at: string
}

/** 丹方草稿（新建 / 编辑出新版本共用的提交内容） */
export interface RecipeDraft {
  name: string
  description?: string
  skill_schema?: SkillSchema
  tags?: string[]
  author?: string
  version_label?: string
}

/** 金丹库存列表项（可用实例；列表恒为 available） */
export interface PillItemListItem {
  id: string
  /** 来源丹方当前版本名称 */
  name: string
  state: PillItemState
  recipe_id: string
  revision_id: string
  revision: number
  created_at: string
}

/** 金丹库存实例详情（任意状态可读；已消耗/弃置展示去向） */
export interface PillItemDetail {
  id: string
  name: string
  description: string
  /** 来源版本标签（hero spotlight 丹性行） */
  tags: string[]
  /** available / consumed_by_agent / consumed_by_fusion / discarded */
  state: PillItemState
  recipe_id: string
  revision_id: string
  revision: number
  version_label: string
  /** 来源丹方已归档时展示 */
  archived_at?: string | null
  /** 已消耗/已弃置时间 */
  consumed_at?: string | null
  created_at: string
}

/** 道人已吸收能力（服用快照；item_id 指向消耗后原实例，revision_id 指向不可变版本） */
export interface AgentEffect {
  id: string
  name: string
  schema: SkillSchema
  weight: number
  sort_order: number
  item_id: string
  revision_id: string
  created_at: string
  removed_at?: string | null
}

/** 幂等写操作统一结果（operation_id 与请求 Idempotency-Key 同值；断线恢复按它查） */
export interface PillOperationResult {
  operation_id: string
  recipe_id?: string
  revision_id?: string
  item_ids?: string[]
  effect_id?: string
  consumed_item_ids?: string[]
}

/** 融合预览（两阶段第一阶段；预览不消耗材料） */
export interface FusionPreview {
  preview_id: string
  expires_at: string
  name: string
  description: string
  skill_schema: SkillSchema
  operator: { id: string; name: string }
  model: string
  degraded: boolean
}

/** 能力列表响应（道人活跃能力，按 sort_order 升序；effects_revision 供 PUT 乐观锁） */
export interface AgentEffectsResponse {
  effects_revision: number
  effects: AgentEffect[]
}

/** 能力全量编排响应（提交集必须等于活跃集；乐观锁由 effects_revision 承担） */
export interface UpdateEffectsResponse {
  effects_revision: number
  effects: AgentEffect[]
}

/** 旧金丹 legacy 解析跳转（GET /pills/:id；旧 ID 不再是可用金丹） */
export interface PillLegacyPointer {
  entity_type: 'recipe'
  recipe_id: string
}

/** 丹方列表查询参数 */
export interface RecipeListParams {
  page?: number
  size?: number
  keyword?: string
  include_archived?: boolean
}

/** 金丹库存列表查询参数 */
export interface PillItemListParams {
  page?: number
  size?: number
  recipe_id?: string
}

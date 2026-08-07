# Data Model: 金丹化性 · Skill-Persona Alchemy Pivot

**Feature Branch**: `001-skill-persona-alchemy-pivot`
**Date**: 2026-08-07

## Entity Overview

| Entity | Table | Purpose | Status |
|--------|-------|---------|--------|
| DaoAgent | `dao_agents` | 道人：基础 AI 人格 | 改造 |
| ElixirPill | `elixir_pills` | 金丹：语言模式/人格特质技能包 | 重度改造 |
| AgentPill | `agent_pills` | 服用记录：道人与金丹的多对多关系 | 改造 |
| LanguagePattern | `language_patterns` | 语言模式缓存：合成后的系统提示词 | 新增 |
| ChatSession | `chat_sessions` | 对话会话 | 基本不变 |
| ChatMessage | `chat_messages` | 对话消息 | 移除 sources 必填 |

---

## Entity: DaoAgent（道人）

**Table**: `dao_agents`

**Purpose**: 代表一个 AI 角色，拥有基础性格和已服用金丹列表。

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | uint | PK, auto_increment | 道人唯一标识 |
| name | string(100) | not null | 道人名称 |
| avatar | string(255) | nullable | 头像 URL |
| personality | text | nullable | 基础性格描述 / 系统提示词 |
| model_name | string(50) | default: gpt-4o | 使用的 LLM 模型 |
| status | string(20) | default: active | active / inactive |
| created_at | timestamp | autoCreateTime | 创建时间 |
| updated_at | timestamp | autoUpdateTime | 更新时间 |

### Relationships

- One-to-Many with `AgentPill` (cascade delete)
- One-to-Many with `ChatSession` (cascade delete)
- One-to-One with `LanguagePattern` (cascade delete, nullable)

### Validation Rules

- `name` 必填，长度 1-100
- `status` 必须是 `active` 或 `inactive`
- `model_name` 长度不超过 50

### State Transitions

- `active` ↔ `inactive`：用户可手动停用/启用道人
- 删除道人时级联删除服用记录、会话、消息、语言模式缓存

---

## Entity: ElixirPill（金丹）

**Table**: `elixir_pills`

**Purpose**: 代表一套可影响语言模式的结构化技能包。基于 nuwa-skill 的 SKILL.md 结构。

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | uint | PK, auto_increment | 金丹唯一标识 |
| name | string(100) | not null | 金丹名称 |
| description | text | nullable | 金丹简介（含触发语、反触发语） |
| skill_schema | jsonb | not null | nuwa-skill 结构化内容（见下方 Schema） |
| tags | jsonb | default: [] | 标签数组，如 ["文言文", "幽默"] |
| author | string(100) | nullable | 作者 |
| version | string(20) | default: 1.0.0 | 版本号 |
| is_builtin | bool | default: false | 是否系统内置示例金丹 |
| created_at | timestamp | autoCreateTime | 创建时间 |
| updated_at | timestamp | autoUpdateTime | 更新时间 |

### Removed Fields (from original RAG design)

- `status`（炼丹状态）
- `vector_count`（向量数量）

### skill_schema JSONB Schema

```json
{
  "identity_card": "string",           // 第一人称身份卡
  "expression_dna": {
    "sentence_length": "short|medium|long|mixed",
    "formality": 0.7,                   // 0-1
    "vocabulary": ["高频词1", "高频词2"],
    "taboo_words": ["禁用词1"],
    "rhythm": "string",
    "humor_type": "string",
    "certainty_style": "string",
    "citation_habit": "string"
  },
  "mental_models": [
    {
      "name": "string",
      "one_liner": "string",
      "source_evidence": ["scene1", "scene2"],
      "application": "string",
      "detection_questions": ["q1"],
      "limitations": ["limit1"]
    }
  ],
  "decision_heuristics": [
    {
      "condition": "string",
      "action": "string",
      "case": "string"
    }
  ],
  "values": ["string"],
  "anti_patterns": ["string"],
  "honest_limits": ["string"],
  "example_dialogues": [
    {
      "user": "string",
      "assistant": "string"
    }
  ],
  "agentic_protocol": {
    "question_classification": "string",
    "research_dimensions": ["string"],
    "answer_rules": ["string"]
  }
}
```

### Validation Rules

- `name` 必填，长度 1-100
- `skill_schema` 必填，必须为合法 JSON
- `skill_schema.expression_dna` 必须存在
- `skill_schema.mental_models` 长度 0-20
- `skill_schema.example_dialogues` 长度 0-10
- `version` 格式建议符合语义化版本（可选校验）

### Relationships

- One-to-Many with `AgentPill` (cascade delete)

### State Transitions

- 金丹创建后为 `is_builtin=false`
- 金丹更新时，所有引用它的 `LanguagePattern` 缓存应标记为失效

---

## Entity: AgentPill（服用记录）

**Table**: `agent_pills`

**Purpose**: 记录道人与金丹的绑定关系，支持权重和顺序。

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | uint | PK, auto_increment | 服用记录唯一标识 |
| agent_id | uint | not null, FK | 道人ID |
| pill_id | uint | not null, FK | 金丹ID |
| weight | float | default: 1.0, min 0, max 10 | 剂量/权重 |
| sort_order | int | default: 0 | 服用顺序 |
| created_at | timestamp | autoCreateTime | 服用时间 |

### Validation Rules

- `(agent_id, pill_id)` 联合唯一
- `weight` 范围 [0, 10]
- `sort_order` ≥ 0

### Relationships

- Many-to-One with `DaoAgent`
- Many-to-One with `ElixirPill`

### State Transitions

- 创建/更新/删除服用记录时，关联道人的 `LanguagePattern` 缓存应失效

---

## Entity: LanguagePattern（语言模式缓存）

**Table**: `language_patterns`

**Purpose**: 缓存每个道人合成后的系统提示词与涌现规则，避免每次对话重复合成。

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | uint | PK, auto_increment | 缓存唯一标识 |
| agent_id | uint | not null, FK, unique | 关联道人ID |
| system_prompt | text | not null | 合成后的系统提示词 |
| emergence_rules | jsonb | default: [] | 涌现规则列表 |
| inner_tensions | jsonb | default: [] | 检测到的内在冲突 |
| source_fingerprint | string(64) | not null | 来源指纹（hash of personality + sorted pills + weights） |
| is_valid | bool | default: true | 是否有效 |
| created_at | timestamp | autoCreateTime | 创建时间 |
| updated_at | timestamp | autoUpdateTime | 更新时间 |

### Validation Rules

- `agent_id` 唯一
- `system_prompt` 必填
- `source_fingerprint` 必填，用于判断缓存是否命中

### Relationships

- One-to-One with `DaoAgent`

### State Transitions

- 创建：当道人首次对话或首次服用金丹时生成
- 失效：当道人性格、服用金丹、金丹内容变化时，将 `is_valid` 设为 false 或删除记录
- 重建：失效后下次对话触发重新合成

---

## Entity: ChatSession（对话会话）

**Table**: `chat_sessions`

**Purpose**: 用户与某个道人之间的对话上下文。

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | uint | PK, auto_increment | 会话唯一标识 |
| agent_id | uint | not null, FK | 关联道人ID |
| title | string(200) | nullable | 会话标题 |
| created_at | timestamp | autoCreateTime | 创建时间 |
| updated_at | timestamp | autoUpdateTime | 更新时间 |

### Relationships

- Many-to-One with `DaoAgent`
- One-to-Many with `ChatMessage` (cascade delete)

### Validation Rules

- `agent_id` 必须指向存在的道人
- `title` 长度不超过 200

---

## Entity: ChatMessage（对话消息）

**Table**: `chat_messages`

**Purpose**: 存储用户与道人的对话内容。

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | uint | PK, auto_increment | 消息唯一标识 |
| session_id | uint | not null, FK | 所属会话ID |
| role | string(20) | not null | user / assistant / system |
| content | text | not null | 消息内容 |
| sources | jsonb | nullable | **废弃/保留为空**：原 RAG 引用来源 |
| created_at | timestamp | autoCreateTime | 创建时间 |

### Removed/Deprecated

- `sources` 字段不再填充，保留 JSONB 列以兼容历史数据。

### Validation Rules

- `role` 必须是 `user`、`assistant` 或 `system`
- `content` 必填
- `session_id` 必须指向存在的会话

---

## ER Diagram (Text)

```text
DaoAgent ||--o{ AgentPill : consumes
DaoAgent ||--o| LanguagePattern : caches
DaoAgent ||--o{ ChatSession : participates
ElixirPill ||--o{ AgentPill : consumed_by
ChatSession ||--o{ ChatMessage : contains
```

## Migration Notes

1. **elixir_pills 表改造**：
   - 删除 `status`、`vector_count` 列
   - 添加 `skill_schema`、`tags`、`author`、`version`、`is_builtin` 列
   - 历史金丹数据：若无法迁移到新 schema，则标记为 `is_builtin=false` 并提供默认空 schema，或提供一次性迁移脚本

2. **agent_pills 表改造**：
   - 添加 `weight`（float，default 1.0）
   - 添加 `sort_order`（int，default 0）

3. **新增 language_patterns 表**：
   - 与 `dao_agents` 一对一
   - 级联删除

4. **elixir_recipes 表**：
   - 删除

5. **chat_messages.sources**：
   - 保留列但不再写入新数据

## Indexes

- `idx_agent_pills_agent_id` on `agent_pills(agent_id)`
- `idx_agent_pills_pill_id` on `agent_pills(pill_id)`
- `idx_chat_messages_session_id` on `chat_messages(session_id)`
- `idx_chat_sessions_agent_id` on `chat_sessions(agent_id)`
- `idx_elixir_pills_is_builtin` on `elixir_pills(is_builtin)`
- `idx_language_patterns_agent_id` on `language_patterns(agent_id)`

## Validation Matrix

| Rule | Layer | Implementation |
|------|-------|----------------|
| 道人名称必填 | Go handler + model tag | `binding:"required,max=100"` |
| 金丹 skill_schema 必填 | Go handler + JSONB check | custom validator |
| 服用记录联合唯一 | DB unique index + service check | `uniqueIndex:idx_agent_pill` |
| 权重范围 [0,10] | Go handler + DB check | `binding:"gte=0,lte=10"` |
| 消息 role 枚举 | Go handler + DB enum | `binding:"oneof=user assistant system"` |
| 缓存指纹一致性 | Python synthesis service | SHA256 of sorted inputs |

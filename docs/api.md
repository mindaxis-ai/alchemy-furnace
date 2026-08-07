# API 接口文档

## 基础信息

- **Base URL**: `/api/v1`
- **请求格式**: JSON
- **响应格式**: JSON
- **WebSocket**: `/api/v1/chat/ws/:session_id`

### 统一响应格式

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

错误响应:

```json
{
  "code": 1001,
  "message": "错误描述",
  "data": null
}
```

## 金丹管理 (Pills)

金丹是语言模式/人格特质的结构化技能包，核心内容为 `skill_schema`（表达 DNA、心智模型、决策启发式、禁忌、诚实边界、示例对话等），详见 [data-model.md](../specs/001-skill-persona-alchemy-pivot/data-model.md)。

### 获取金丹列表

```
GET /api/v1/pills?page=1&page_size=10&keyword=文言&is_builtin=true
```

查询参数：

| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码，默认 1 |
| page_size | int | 每页数量，默认 10，最大 100 |
| keyword | string | 可选，搜索名称/描述 |
| is_builtin | bool | 可选，筛选内置金丹 |

响应:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 1,
        "name": "文言文金丹",
        "description": "令回复带有文言色彩",
        "tags": ["文言文", "古雅"],
        "author": "系统",
        "version": "1.0.0",
        "is_builtin": true,
        "created_at": "2026-08-07T10:00:00Z",
        "updated_at": "2026-08-07T10:00:00Z"
      }
    ],
    "total": 50,
    "page": 1,
    "page_size": 10
  }
}
```

### 创建金丹

```
POST /api/v1/pills
```

请求体:
```json
{
  "name": "文言文金丹",
  "description": "令道人开口便是之乎者也",
  "skill_schema": {
    "identity_card": "我是一位熟读经史的古人，说话喜用文言。",
    "expression_dna": {
      "sentence_length": "medium",
      "formality": 0.9,
      "vocabulary": ["之", "乎", "者", "也"],
      "taboo_words": ["你", "我", "的", "了"]
    },
    "mental_models": [],
    "decision_heuristics": [],
    "honest_limits": [],
    "example_dialogues": []
  },
  "tags": ["文言文", "古雅"],
  "author": "用户A",
  "version": "1.0.0"
}
```

> `skill_schema.expression_dna` 必填；更新金丹后，所有引用它的道人语言模式缓存自动失效。

### 获取金丹详情

```
GET /api/v1/pills/:id
```

响应中包含完整的 `skill_schema`。

### 更新金丹

```
PUT /api/v1/pills/:id
```

请求体同创建（字段可选）。

### 删除金丹

```
DELETE /api/v1/pills/:id
```

> 级联删除该金丹的所有服用记录，并使相关道人的语言模式缓存失效。

## 道人管理 (Agents)

### 获取道人列表

```
GET /api/v1/agents?page=1&page_size=10&status=active
```

`status` 可选：`active` | `inactive`。

### 创建道人

```
POST /api/v1/agents
```

请求体:
```json
{
  "name": "太上老君",
  "avatar": "https://example.com/avatar.png",
  "personality": "你是太上老君，道家的始祖，言谈充满智慧...",
  "model_name": "gpt-4o"
}
```

### 获取道人详情

```
GET /api/v1/agents/:id
```

响应中包含已服用金丹列表（含 weight/sort_order）与当前语言模式缓存状态：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 1,
    "name": "太上老君",
    "personality": "...",
    "model_name": "gpt-4o",
    "status": "active",
    "pills": [
      { "id": 2, "name": "文言文金丹", "weight": 1.0, "sort_order": 0 }
    ],
    "language_pattern": {
      "is_valid": true,
      "system_prompt": "...",
      "emergence_rules": [],
      "inner_tensions": []
    }
  }
}
```

### 更新道人

```
PUT /api/v1/agents/:id
```

> 修改 `personality` 会使语言模式缓存失效。

### 删除道人

```
DELETE /api/v1/agents/:id
```

> 级联删除服用记录、会话、消息与语言模式缓存。

### 服用金丹（绑定）

```
POST /api/v1/agents/:id/pills
```

请求体:
```json
{
  "pill_id": 2,
  "weight": 1.5,
  "sort_order": 1
}
```

> `(agent_id, pill_id)` 联合唯一；`weight` 范围 [0, 10]；绑定后语言模式缓存失效。

### 更新服用记录（权重/顺序）

```
PUT /api/v1/agents/:id/pills/:pill_id
```

请求体:
```json
{
  "weight": 2.0,
  "sort_order": 0
}
```

### 解除金丹绑定

```
DELETE /api/v1/agents/:id/pills/:pill_id
```

### 获取已服用金丹列表

```
GET /api/v1/agents/:id/pills
```

## 试丹 (Trial)

临时组合性格与金丹，不绑定道人即可预览合成效果，不落库。

### 合成预览

```
POST /api/v1/trial/synthesis
```

请求体:
```json
{
  "personality": "沉稳内敛",
  "pills": [
    { "pill_id": 2, "weight": 1.0, "sort_order": 0 }
  ],
  "model_name": "gpt-4o-mini"
}
```

响应:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "system_prompt": "...",
    "emergence_rules": ["..."],
    "inner_tensions": []
  }
}
```

### 试丹对话（非流式）

```
POST /api/v1/trial/chat
```

请求体:
```json
{
  "personality": "沉稳内敛",
  "pills": [
    { "pill_id": 2, "weight": 1.0, "sort_order": 0 }
  ],
  "messages": [
    { "role": "user", "content": "何为道？" }
  ],
  "model": "gpt-4o",
  "temperature": 0.7,
  "max_tokens": 4096
}
```

响应:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "content": "道者，万物之奥也...",
    "model": "gpt-4o",
    "usage": { "prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150 }
  }
}
```

## 对话管理 (Chat)

### 创建会话

```
POST /api/v1/chat/sessions
```

请求体:
```json
{
  "agent_id": 1,
  "title": "论道"
}
```

### 获取会话列表

```
GET /api/v1/chat/sessions?page=1&page_size=20
```

### 获取会话消息

```
GET /api/v1/chat/sessions/:id/messages?page=1&page_size=50
```

响应:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "list": [
      {
        "id": 1,
        "session_id": 1,
        "role": "user",
        "content": "何为道？",
        "created_at": "2026-08-07T10:00:00Z"
      },
      {
        "id": 2,
        "session_id": 1,
        "role": "assistant",
        "content": "道可道，非常道...",
        "created_at": "2026-08-07T10:00:01Z"
      }
    ],
    "total": 10,
    "page": 1,
    "page_size": 50
  }
}
```

### 流式对话 (WebSocket)

```
WS /api/v1/chat/ws/:session_id
```

连接后发送 JSON 消息:
```json
{
  "content": "何为道？"
}
```

服务端流式返回:
```json
{"type": "chunk", "content": "道"}
{"type": "chunk", "content": "可"}
{"type": "chunk", "content": "道"}
...
{"type": "done"}
```

错误返回:
```json
{"type": "error", "message": "错误描述"}
```

## 系统接口 (System)

### 健康检查

```
GET /api/v1/system/health
```

响应:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "healthy",
    "version": "1.0.0",
    "services": {
      "database": "connected",
      "python_engine": "connected"
    }
  }
}
```

### 获取系统配置

```
GET /api/v1/system/config
```

响应:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "models": [
      {"id": "gpt-4o", "name": "GPT-4o"},
      {"id": "gpt-4o-mini", "name": "GPT-4o Mini"},
      {"id": "deepseek-chat", "name": "DeepSeek"}
    ],
    "default_model": "gpt-4o",
    "synthesis_model": "gpt-4o-mini"
  }
}
```

## 错误码

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 1001 | 参数错误 |
| 1002 | 未找到资源 |
| 1003 | 权限不足 |
| 3001 | 语言引擎调用失败 |
| 3002 | 语言模式合成失败 |
| 4001 | LLM 调用失败 |
| 5001 | 数据库错误 |
| 5002 | WebSocket 错误 |
| 9999 | 系统内部错误 |

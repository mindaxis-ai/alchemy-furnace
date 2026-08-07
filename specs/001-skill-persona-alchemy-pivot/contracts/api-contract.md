# API Contract: Public REST Endpoints

**Base URL**: `/api/v1`
**Format**: JSON
**Auth**: TBD (currently no auth in codebase; assume same as existing)

---

## DaoAgents（道人）

### List Agents

```text
GET /api/v1/agents
```

**Query Parameters**:
- `page` (int, default 1)
- `page_size` (int, default 10, max 100)
- `status` (string, optional): `active` | `inactive`

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": 1,
        "name": "清风道长",
        "avatar": "...",
        "personality": "沉稳内敛，喜好引经据典",
        "model_name": "gpt-4o",
        "status": "active",
        "created_at": "2026-08-07T10:00:00Z",
        "updated_at": "2026-08-07T10:00:00Z"
      }
    ],
    "total": 100,
    "page": 1,
    "page_size": 10
  }
}
```

### Create Agent

```text
POST /api/v1/agents
```

**Request Body**:
```json
{
  "name": "清风道长",
  "avatar": "https://example.com/avatar.png",
  "personality": "沉稳内敛，喜好引经据典",
  "model_name": "gpt-4o"
}
```

**Response 201**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "name": "清风道长",
    "avatar": "...",
    "personality": "沉稳内敛，喜好引经据典",
    "model_name": "gpt-4o",
    "status": "active",
    "created_at": "2026-08-07T10:00:00Z",
    "updated_at": "2026-08-07T10:00:00Z"
  }
}
```

### Get Agent

```text
GET /api/v1/agents/:id
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "name": "清风道长",
    "avatar": "...",
    "personality": "沉稳内敛，喜好引经据典",
    "model_name": "gpt-4o",
    "status": "active",
    "pills": [
      {
        "id": 2,
        "name": "文言文金丹",
        "weight": 1.0,
        "sort_order": 0
      }
    ],
    "language_pattern": {
      "is_valid": true,
      "system_prompt": "...",
      "emergence_rules": ["..."],
      "inner_tensions": []
    },
    "created_at": "2026-08-07T10:00:00Z",
    "updated_at": "2026-08-07T10:00:00Z"
  }
}
```

### Update Agent

```text
PUT /api/v1/agents/:id
```

**Request Body**:
```json
{
  "name": "清风道长（改）",
  "personality": "沉稳内敛，喜好引经据典，偶尔幽默",
  "model_name": "gpt-4o"
}
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": { /* updated agent */ }
}
```

### Delete Agent

```text
DELETE /api/v1/agents/:id
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

## Agent Pills（服用金丹）

### Bind Pill

```text
POST /api/v1/agents/:id/pills
```

**Request Body**:
```json
{
  "pill_id": 2,
  "weight": 1.5,
  "sort_order": 1
}
```

**Response 201**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 10,
    "agent_id": 1,
    "pill_id": 2,
    "weight": 1.5,
    "sort_order": 1,
    "created_at": "2026-08-07T10:00:00Z"
  }
}
```

### Update Bound Pill

```text
PUT /api/v1/agents/:id/pills/:pill_id
```

**Request Body**:
```json
{
  "weight": 2.0,
  "sort_order": 0
}
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": { /* updated agent_pill */ }
}
```

### Unbind Pill

```text
DELETE /api/v1/agents/:id/pills/:pill_id
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

## ElixirPills（金丹）

### List Pills

```text
GET /api/v1/pills
```

**Query Parameters**:
- `page` (int, default 1)
- `page_size` (int, default 10, max 100)
- `keyword` (string, optional): 搜索名称/描述
- `is_builtin` (bool, optional): 是否内置

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": 2,
        "name": "文言文金丹",
        "description": "令回复带有文言色彩...",
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

### Get Pill

```text
GET /api/v1/pills/:id
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 2,
    "name": "文言文金丹",
    "description": "令回复带有文言色彩...",
    "skill_schema": { /* nuwa-skill schema */ },
    "tags": ["文言文", "古雅"],
    "author": "系统",
    "version": "1.0.0",
    "is_builtin": true,
    "created_at": "2026-08-07T10:00:00Z",
    "updated_at": "2026-08-07T10:00:00Z"
  }
}
```

### Create Pill

```text
POST /api/v1/pills
```

**Request Body**:
```json
{
  "name": "赛博朋克金丹",
  "description": "让道人用词充满霓虹、芯片、义体意象",
  "skill_schema": {
    "identity_card": "我是一名从 2077 年穿越而来的流浪黑客...",
    "expression_dna": {
      "sentence_length": "short",
      "formality": 0.3,
      "vocabulary": ["芯片", "霓虹", "义体", "接口"]
    },
    "mental_models": [...],
    "example_dialogues": [...]
  },
  "tags": ["科幻", "赛博朋克"],
  "author": "用户A",
  "version": "1.0.0"
}
```

**Response 201**:
```json
{
  "code": 0,
  "message": "success",
  "data": { /* created pill */ }
}
```

### Update Pill

```text
PUT /api/v1/pills/:id
```

**Request Body**: 同 Create Pill（字段可选）

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": { /* updated pill */ }
}
```

### Delete Pill

```text
DELETE /api/v1/pills/:id
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

## Trial（试丹）

### Preview Language Pattern

```text
POST /api/v1/trial/synthesis
```

**Request Body**:
```json
{
  "personality": "沉稳内敛",
  "pills": [
    {
      "pill_id": 2,
      "weight": 1.0,
      "sort_order": 0
    }
  ],
  "model_name": "gpt-4o-mini"
}
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "system_prompt": "...",
    "emergence_rules": ["..."],
    "inner_tensions": []
  }
}
```

### Trial Chat (Non-streaming)

```text
POST /api/v1/trial/chat
```

**Request Body**:
```json
{
  "personality": "沉稳内敛",
  "pills": [...],
  "messages": [
    { "role": "user", "content": "你好" }
  ],
  "model": "gpt-4o",
  "temperature": 0.7,
  "max_tokens": 4096
}
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "content": "...",
    "model": "gpt-4o",
    "usage": { "prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150 }
  }
}
```

---

## Chat Sessions（会话）

Endpoints remain largely unchanged except `sources` is no longer returned.

### List Sessions

```text
GET /api/v1/sessions
```

### Create Session

```text
POST /api/v1/sessions
```

**Request Body**:
```json
{
  "agent_id": 1,
  "title": "与清风道长的论道"
}
```

### Get Session

```text
GET /api/v1/sessions/:id
```

### Get Messages

```text
GET /api/v1/sessions/:id/messages
```

**Response 200**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [
      {
        "id": 1,
        "session_id": 1,
        "role": "assistant",
        "content": "...",
        "created_at": "2026-08-07T10:00:00Z"
      }
    ],
    "total": 10,
    "page": 1,
    "page_size": 20
  }
}
```

### WebSocket Chat

```text
WS /api/v1/chat/:session_id/stream
```

See [websocket-contract.md](websocket-contract.md) for streaming protocol details.

---

## Error Response

All errors follow the existing response format:

```json
{
  "code": 1001,
  "message": "道人不存在",
  "detail": "agent id=999 not found"
}
```

## Common HTTP Status Codes

| Status | Meaning |
|--------|---------|
| 200 | OK |
| 201 | Created |
| 400 | Bad Request |
| 404 | Not Found |
| 500 | Internal Server Error |

**Note**: The existing codebase returns 200 for most operations with `code` in body. New endpoints should follow the same convention unless intentionally modernizing.

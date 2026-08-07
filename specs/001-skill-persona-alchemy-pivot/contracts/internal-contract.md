# Internal Contract: Go API Gateway ↔ Python Language Engine

**Transport**: HTTP/JSON
**Base URL (Go → Python)**: configured via `PYTHON_RAG_BASE_URL` (renamed to `PYTHON_ENGINE_BASE_URL` in config)
**Timeout**: synthesis 30s, chat 120s

---

## Synthesis Service

### Combine Personality + Pills

```text
POST /api/v1/synthesis/combine
```

**Request Body**:
```json
{
  "personality": "沉稳内敛，喜好引经据典",
  "pills": [
    {
      "id": 2,
      "name": "文言文金丹",
      "weight": 1.0,
      "sort_order": 0,
      "skill_schema": {
        "identity_card": "...",
        "expression_dna": { ... },
        "mental_models": [ ... ],
        "decision_heuristics": [ ... ],
        "values": [ ... ],
        "anti_patterns": [ ... ],
        "honest_limits": [ ... ],
        "example_dialogues": [ ... ]
      }
    }
  ],
  "model": "gpt-4o-mini",
  "temperature": 0.7,
  "max_tokens": 2048
}
```

**Response Body (200)**:
```json
{
  "system_prompt": "string",
  "emergence_rules": [
    "融合后的规则 1",
    "融合后的规则 2"
  ],
  "inner_tensions": [
    {
      "dimension": "sentence_length",
      "description": "沉稳性格偏好长句，嘻哈金丹偏好短句",
      "severity": "medium"
    }
  ],
  "fingerprint": "sha256:abcdef...",
  "model": "gpt-4o-mini",
  "usage": {
    "prompt_tokens": 800,
    "completion_tokens": 200,
    "total_tokens": 1000
  }
}
```

**Error Response (500)**:
```json
{
  "error": "Synthesis failed: ..."
}
```

**Behavior**:
- Python service performs structured merge + LLM emergence derivation.
- Returns a stable `fingerprint` that Go can use for cache invalidation.
- If `pills` is empty, returns a system prompt based only on `personality`.

---

## Chat Service

### Non-streaming Chat

```text
POST /api/v1/chat/completions
```

**Request Body**:
```json
{
  "messages": [
    { "role": "system", "content": "... synthesized system prompt ..." },
    { "role": "user", "content": "你好" }
  ],
  "model": "gpt-4o",
  "temperature": 0.7,
  "max_tokens": 4096
}
```

**Response Body (200)**:
```json
{
  "content": "...",
  "model": "gpt-4o",
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "total_tokens": 150
  }
}
```

### Streaming Chat

```text
POST /api/v1/chat/completions/stream
```

**Request Body**: 同 Non-streaming Chat

**Response**: `text/event-stream`

```text
data: {"content": "你"}

data: {"content": "好"}

data: [DONE]
```

**Error Handling**:
- On error, yield `data: {"error": "..."}` followed by `data: [DONE]`.

---

## Quality Check Service (Optional V1)

### Validate Pill Schema

```text
POST /api/v1/quality/validate-pill
```

**Request Body**:
```json
{
  "skill_schema": { ... }
}
```

**Response Body (200)**:
```json
{
  "valid": true,
  "score": 85,
  "issues": []
}
```

Or:

```json
{
  "valid": false,
  "score": 55,
  "issues": [
    "缺少 honest_limits",
    "mental_models 数量不足 3 个"
  ]
}
```

---

## Configuration Contract

Go config must expose:

```yaml
python_engine:
  base_url: "http://python:8000"
llm:
  default_model: "gpt-4o"
  synthesis_model: "gpt-4o-mini"  # for cheaper pattern synthesis
```

Python config must expose:

```yaml
OPENAI_API_KEY: "..."
OPENAI_BASE_URL: "..."
DEFAULT_MODEL: "gpt-4o"
SYNTHESIS_MODEL: "gpt-4o-mini"
```

---

## Health Check

```text
GET /api/v1/health
```

**Response Body (200)**:
```json
{
  "status": "ok",
  "version": "2.0.0",
  "components": {
    "openai": "ok",
    "database": "not_applicable"
  }
}
```

**Note**: Remove `qdrant` from health check components.

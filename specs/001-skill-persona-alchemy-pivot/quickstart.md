# Quickstart: 金丹化性 · Skill-Persona Alchemy Pivot

**Feature Branch**: `001-skill-persona-alchemy-pivot`
**Date**: 2026-08-07

This guide helps developers run and validate the new skill-persona alchemy system locally.

## Prerequisites

- Docker 20.10+
- Docker Compose 2.0+
- OpenAI-compatible API key
- Node.js 18+ (for frontend development)
- Go 1.21+ (for backend development)
- Python 3.11+ (for language engine development)

## 1. Clone and Checkout

```bash
git clone https://github.com/yusanwen-code/alchemy-furnace.git
cd alchemy-furnace
git checkout 001-skill-persona-alchemy-pivot
```

## 2. Configure Environment

```bash
cp .env.example .env
```

Edit `.env`:

```env
# OpenAI-compatible API
OPENAI_API_KEY=your-api-key
OPENAI_BASE_URL=https://api.openai.com/v1
DEFAULT_MODEL=gpt-4o
SYNTHESIS_MODEL=gpt-4o-mini

# Database
POSTGRES_USER=alchemy
POSTGRES_PASSWORD=alchemy
POSTGRES_DB=alchemy

# Go config
GIN_MODE=debug
PYTHON_ENGINE_BASE_URL=http://python-engine:8000
```

## 3. Start Services

```bash
# Build and start (Qdrant removed)
docker-compose up --build

# Or use Make
make dev
```

Services:
- Frontend: http://localhost
- Go API: http://localhost:8080
- Python Engine: http://localhost:8000
- PostgreSQL: localhost:5432

## 4. Verify Health

```bash
curl http://localhost:8080/api/v1/system/health
curl http://localhost:8000/api/v1/health
```

Expected: no `qdrant` component; `openai` status `ok`.

## 5. Create a Taoist (道人)

```bash
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "清风道长",
    "personality": "沉稳内敛，喜好引经据典",
    "model_name": "gpt-4o"
  }'
```

## 6. Create a Skill Pill (金丹)

```bash
curl -X POST http://localhost:8080/api/v1/pills \
  -H "Content-Type: application/json" \
  -d '{
    "name": "文言文金丹",
    "description": "令道人开口便是之乎者也",
    "skill_schema": {
      "identity_card": "我是一位熟读经史的古人，说话喜用文言。",
      "expression_dna": {
        "sentence_length": "medium",
        "formality": 0.9,
        "vocabulary": ["之", "乎", "者", "也", "汝", "吾"],
        "taboo_words": ["你", "我", "的", "了"]
      },
      "mental_models": [
        {
          "name": "以古喻今",
          "one_liner": "用典籍中的道理回应现代问题",
          "source_evidence": ["子曰：学而时习之"],
          "application": "回答前先想一句古文",
          "detection_questions": ["此事古人如何看？"],
          "limitations": ["无法引用 20 世纪后的文献"]
        }
      ],
      "decision_heuristics": [
        {
          "condition": "用户提问涉及现代概念",
          "action": "用古代类比解释",
          "case": "解释互联网时说‘此网如丝绸之路’"
        }
      ],
      "values": ["典雅", "含蓄"],
      "anti_patterns": ["使用网络流行语", "直白口语"],
      "honest_limits": ["不能真正引用未读过的典籍", "对现代技术细节可能不准确"],
      "example_dialogues": [
        {
          "user": "什么是人工智能？",
          "assistant": "人工智能者，乃人造之灵智也。虽无血肉，却能推演万象，犹师旷之耳，扁鹊之眼。"
        }
      ]
    },
    "tags": ["文言文", "古雅"],
    "author": "system",
    "version": "1.0.0"
  }'
```

## 7. Consume Pill

```bash
# agent_id=1, pill_id=1
curl -X POST http://localhost:8080/api/v1/agents/1/pills \
  -H "Content-Type: application/json" \
  -d '{
    "pill_id": 1,
    "weight": 1.0,
    "sort_order": 0
  }'
```

## 8. Test Synthesis

```bash
curl -X POST http://localhost:8080/api/v1/trial/synthesis \
  -H "Content-Type: application/json" \
  -d '{
    "personality": "沉稳内敛，喜好引经据典",
    "pills": [
      {
        "pill_id": 1,
        "weight": 1.0,
        "sort_order": 0
      }
    ],
    "model_name": "gpt-4o-mini"
  }'
```

## 9. Chat

### Non-streaming

```bash
curl -X POST http://localhost:8080/api/v1/trial/chat \
  -H "Content-Type: application/json" \
  -d '{
    "personality": "沉稳内敛，喜好引经据典",
    "pills": [
      {
        "pill_id": 1,
        "weight": 1.0,
        "sort_order": 0
      }
    ],
    "messages": [
      { "role": "user", "content": "你好" }
    ],
    "model": "gpt-4o"
  }'
```

### WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/chat/ws/1');
ws.onmessage = (event) => {
  console.log(JSON.parse(event.data));
};
ws.onopen = () => {
  ws.send(JSON.stringify({ content: '你好' }));
};
```

## 10. Combine Multiple Pills

Create a second pill (e.g., "Cyberpunk Pill") and bind it to the same agent:

```bash
curl -X POST http://localhost:8080/api/v1/agents/1/pills \
  -H "Content-Type: application/json" \
  -d '{
    "pill_id": 2,
    "weight": 1.2,
    "sort_order": 1
  }'
```

Then chat again. The reply should show blended traits from both pills.

## 11. Run Tests

```bash
# Go tests
make test-go

# Python tests
make test-python

# Frontend tests
make test-frontend
```

## 12. Common Issues

| Issue | Solution |
|-------|----------|
| Python engine fails to start | Check `OPENAI_API_KEY` and `OPENAI_BASE_URL` |
| Synthesis returns generic reply | Verify `skill_schema.expression_dna` and `example_dialogues` are filled |
| Multiple pills don't blend | Check weights and ensure `sort_order` is set |
| Old RAG endpoints still accessible | Verify recipe/upload handlers are removed |

## 13. Cleanup

```bash
docker-compose down -v
```

This removes containers and PostgreSQL data volumes.

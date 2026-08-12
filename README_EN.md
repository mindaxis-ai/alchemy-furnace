# Alchemy Furnace · 炼丹炉

[中文](./README.md) · English

> **Fire as the catalyst, herbs as the base.** Watch the spirit flow, watch the elixir form — all things can be refined into golden pills.

Alchemy Furnace is a playground for "language-mode engineering" of AI Agents. Here, an **Elixir Pill** is a structured skill package that encapsulates a persona and writing style, while a **Dao Agent** is an AI Agent that "takes" a pill — once ingested, the agent's voice is shaped by the pill's essence, speaking in a distinctly new register.

Under the hood: a Go API gateway, a Python synthesis engine, and a Next.js frontend. Multiple OpenAI-compatible model providers are supported.

---

## Features

**Core**
- **Pill Workshop** — Structured skill editor: expression DNA, mental models, decision heuristics, taboo words, example dialogues
- **Agent Hall** — Create and manage multiple AI Agents, each bound to a chosen LLM
- **Bind Pill** — One agent can ingest multiple pills, with weight and intake-order control
- **Pill Essence Synthesis** — Engine merges pills by weight, deduplicates, detects conflicts, then an LLM call distills a unified system prompt
- **Trial Synthesis** — Preview the synthesis without binding any agent
- **Pill Fusion** — Drop any N pills (N ≥ 2) into the Fusion Furnace. A random mutation operator (Promptbreeder-style) is sampled, and the LLM freely forges a brand-new pill. Lineage is traced; save only when satisfied.
- **Streaming Discourse** — SSE-based chat; the synthesized prompt is always live

**Engineering**
- **Three-tier architecture** — Go gateway + Python engine + Next.js frontend, cleanly separated
- **Responsive UI** — Same code serves desktop and mobile H5
- **Provider-agnostic** — Built-in templates for common OpenAI-compatible providers (DeepSeek, Qwen, Zhipu GLM, Kimi, Baichuan, Wenxin, Ollama); API keys encrypted at rest
- **Synthesis cache** — Each agent's synthesized prompt is cached and rebuilt on personality or pill changes
- **Demo Mode** — `DEMO_MODE=true` runs everything in-memory; no PostgreSQL required

---

## Architecture

```
             Browser / H5
                 │
              Nginx
                 │
         ┌───────┴───────┐
         │               │
       Go API          Python
     (Gin + GORM)    (FastAPI)
         │               │
         └───────┬───────┘
                 │
             PostgreSQL
```

| Service | Stack | Role | Port |
|------|--------|------|------|
| Frontend | Next.js 16 + React 19 + Tailwind 4 | UI (zh-CN / en / responsive) | 3000 |
| Go API | Go 1.21+ + Gin + GORM | Gateway, agents / pills / sessions persistence | 8080 |
| Python Engine | Python 3.11+ + FastAPI | Language synthesis, LLM calls | 8000 |
| PostgreSQL | 14+ | Business data | 5432 |
| Nginx | — | Reverse proxy, static files | 80 |

---

## Quick Start

### 1. One-command Docker deployment

```bash
# 1. Clone
git clone https://github.com/yusanwen-code/alchemy-furnace.git
cd alchemy-furnace

# 2. Configure env
cp .env.example .env
# Edit .env: set OPENAI_API_KEY and MODEL_KEY_SECRET
# MODEL_KEY_SECRET encrypts stored model API keys; generate with: openssl rand -hex 32

# 3. Launch
make deploy         # equivalent to: make init && make build && make up
make ps             # container status
make logs           # tail logs
```

Open http://localhost in your browser.

### 2. Local development (no Docker)

You need: PostgreSQL 14+ (try [Postgres.app](https://postgresapp.com/)), Go 1.21+, Python 3.11+, Node.js 20+.

```bash
# 1. Prepare the database
createuser alchemy && createdb -O alchemy alchemy_db
psql -c "ALTER USER alchemy PASSWORD 'alchemy123';"

# 2. Configure .env
cp .env.example .env
# Set DB_HOST=localhost and PYTHON_ENGINE_BASE_URL=http://localhost:8000

# 3. Launch in three terminals
make dev-python     # Python engine → http://localhost:8000
make dev-go         # Go gateway (auto-migrates + seeds built-in pills) → http://localhost:8080
make dev-front      # Next.js frontend → http://localhost:3000
```

Or run `bash scripts/dev.sh` to bring all three up at once (it handles Docker postgres vs Postgres.app port conflicts).

### 3. Demo Mode

Demo Mode needs **no PostgreSQL** — Go and Python both run in-memory mocks. Ideal for product demos and offline exploration.

```bash
# Local dev
export DEMO_MODE=true
bash scripts/dev.sh

# One-command server deploy
sudo bash scripts/deploy-demo.sh
```

On startup, 9 pills / 9 agents / 9 providers / 9 models / 9 sessions are auto-loaded; chat and synthesis return fixed Chinese replies from a mock provider. See [docs/operations/deploy.md](docs/operations/deploy.md).

---

## First Run

1. **Refine a Pill** — Open "Pill Workshop", create a pill: fill in expression DNA (sentence length, formality, vocabulary, taboo words), mental models, example dialogues
2. **Cultivate an Agent** — Open "Agent Hall", click "Recruit Disciple", set a base personality and a chosen LLM
3. **Bind Pills** — On the agent's detail page, select pills for the agent to ingest; tune weight and order
4. **Trial Synthesis** — Open "Trial Synthesis" to combine personality and pills temporarily and preview the result
5. **Open the Discourse** — Open "Discourse", pick an agent, start the SSE streaming chat

---

## Core Concepts

### Elixir Pill

A structured skill package that defines a language style and persona. Each pill contains:

| Field | Description |
|------|------|
| `identity_card` | Self-concept of the persona this pill embodies |
| `expression_dna` | Expression DNA: sentence length, formality, vocabulary, taboo words |
| `mental_models` | Frameworks for understanding problems |
| `decision_heuristics` | Expression strategies for specific situations |
| `values` / `anti_patterns` | Values and anti-patterns |
| `honest_limits` | Honest declaration of capability limits |
| `example_dialogues` | Concrete demonstrations of the style |

### Dao Agent

An AI Agent entity with a base personality, a chosen LLM, and the pills it has ingested. Its reply style is shaped by "base personality + synthesized pill essence".

### Bind Pill

Associates a pill with an agent, configured by **weight** and **intake order** (sort_order). One agent can ingest many pills at once.

### Synthesis (Language Pattern Synthesis)

Merge pills by weight, deduplicate, detect conflicts, then let an LLM distill a unified system prompt. The flow maps to five alchemical steps:

| Alchemy | Synthesis | Description |
|------|------|------|
| Gather herbs | Read traits | Read the agent's base personality and bound pills |
| Compound | Structured merge | Blend by weight, deduplicate, detect conflicts |
| Kindle | Emergent distillation | LLM call extracts the unified essence and emergent rules |
| Temper | Cache | The synthesized prompt is cached per-agent; rebuilt on change |
| Unseal | Discourse | Stream LLM replies using the synthesized prompt |

### Fusion (Pill Fusion)

Drop any N pills (N ≥ 2) into the Fusion Furnace and let the LLM freely forge a brand-new pill.

**Flow**: pick pills → light the furnace (animation) → preview (with operator & lineage) → re-roll / edit / save.

**Randomness** comes from sampling one of 7 fusion operators (temperature 1.0) per call:

| Operator | Effect |
|---|---|
| Hyperbole | Push traits to the extreme; double the style density |
| Distillation | Strip away; keep only the deepest common ground |
| Dialectic | Let the inner contradictions become the new persona's tension engine |
| Inversion | Flip the core stance; a familiar stranger |
| Dilution | One pill dominates (~70%); the rest are spices |
| Recombination | Field-level crossover across pills |
| Emergent | Don't blend — imagine the next generation raised by all of them |

The new pill's `skill_schema.fusion_lineage` records its parents, operator, and timestamp; the detail page can trace the lineage.

**Technical references**: [Promptbreeder](https://arxiv.org/abs/2309.16797) (LLM as a mutation-operator executor), [Blended Skill Talk](https://arxiv.org/abs/2004.08449) (multiple skills fused into a single coherent persona), [EvoPrompt](https://arxiv.org/abs/2309.08532) (LLM-driven prompt crossover).

---

## API Quick Reference

```bash
# Create a pill
curl -X POST http://localhost:8080/api/v1/pills \
  -H "Content-Type: application/json" \
  -d '{"name": "Classical Chinese Pill", "description": "Makes the agent speak in literary Chinese", "skill_schema": {...}, "tags": ["classical"], "author": "system", "version": "1.0.0"}'

# Create an agent
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "Laozi", "personality": "Daoist founder, full of supreme wisdom", "model_name": "gpt-4o"}'

# Bind a pill
curl -X POST http://localhost:8080/api/v1/agents/{agent_uuid}/pills \
  -H "Content-Type: application/json" \
  -d '{"pill_id": "{pill_uuid}", "weight": 1.0, "sort_order": 0}'

# SSE streaming chat
curl -N -X POST http://localhost:8080/api/v1/chat/sse/{session_uuid} \
  -H "Content-Type: application/json" \
  -d '{"content": "What is the Dao?"}'

# Pill fusion preview (N >= 2 pills -> new pill, NOT persisted)
curl -X POST http://localhost:8080/api/v1/fusion/fuse \
  -H "Content-Type: application/json" \
  -d '{"pill_uuids": ["<uuid1>", "<uuid2>"]}'
```

Full reference: [docs/api.md](docs/api.md).

---

## Documentation

| Document | Description |
|------|------|
| [docs/architecture.md](docs/architecture.md) | System architecture in depth |
| [docs/api.md](docs/api.md) | Full API reference |
| [docs/frontend.md](docs/frontend.md) | Frontend dev guide |
| [docs/operations/deploy.md](docs/operations/deploy.md) | Demo Mode and server deploy |

---

## Project Structure

```
alchemy-furnace/
├── README.md                    # 中文
├── README_EN.md                 # this file
├── Makefile                     # common commands
├── docker-compose.yml
├── .env.example
├── docs/                        # maintenance docs
├── backend/
│   ├── go/                      # Go API gateway (Gin + GORM)
│   │   ├── cmd/                 # entry
│   │   ├── internal/            # handler / service / dao
│   │   └── Dockerfile
│   └── python/                  # Python engine (FastAPI)
│       ├── app/
│       │   ├── api/             # routes
│       │   ├── core/            # config / runtime
│       │   ├── models/          # data models
│       │   └── services/        # synthesis / provider
│       └── Dockerfile
└── frontend/                    # Next.js frontend
    ├── app/                     # App Router ([locale] + (main))
    ├── components/              # business components
    ├── contexts/                # React Context
    ├── lib/                     # utilities
    ├── messages/                # i18n message dictionary
    ├── public/                  # static assets
    └── Dockerfile
```

---

## Tech Stack

**Frontend** · Next.js 16 · React 19 · Tailwind 4 · next-intl · pnpm
**Gateway** · Go 1.21+ · Gin · GORM
**Engine** · Python 3.11+ · FastAPI · OpenAI SDK
**Storage** · PostgreSQL 14+
**Deploy** · Docker Compose · Nginx

---

## License

Custom license — see [LICENSE](LICENSE).

**Summary**
- Personal use, learning, research: free to use, modify, distribute
- Commercial use: must retain the project name "Alchemy Furnace (炼丹炉)" and attribution to "yusanwen-code"; contact the author for a commercial license

---

<p align="center"><i>The Dao follows nature · Forging wisdom in the furnace</i></p>
<p align="center">Made with ☯️ by <a href="https://github.com/yusanwen-code">yusanwen-code</a></p>

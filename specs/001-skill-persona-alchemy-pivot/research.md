# Research: 金丹化性 · Skill-Persona Alchemy Pivot

**Date**: 2026-08-07
**Feature Branch**: `001-skill-persona-alchemy-pivot`

## Research Questions

1. What is [nuwa-skill](https://github.com/alchaincyf/nuwa-skill) and how can its skill format be reused?
2. How should multiple skills/personas be composed to produce emergent language changes?
3. What is the best architecture for the language-pattern synthesis engine?
4. Which existing project components can be reused, and which must be removed?
5. How do we validate that skill fusion produces recognizable, stable language patterns?

---

## Decision: nuwa-skill as the Skill Schema, Not the Runtime

**Decision**: Adopt the nuwa-skill `SKILL.md` structure as the canonical schema for an `ElixirPill` (金丹), but persist it in PostgreSQL JSONB rather than as files. Do not reuse nuwa-skill as a runtime engine.

**Rationale**:
- nuwa-skill is a prompt-only persona-distillation system. It defines a high-quality skill artifact format (YAML frontmatter + markdown sections) but provides no server, no database, and no multi-skill fusion logic.
- Its schema is excellent for describing *how* a person thinks and speaks: expression DNA, mental models, decision heuristics, taboos, honest limits, example dialogues.
- Our project already has a Go/Python/React runtime and an Agent-Pill relationship model. We need a runtime skill format that fits this stack.

**Alternatives considered**:
- *Use nuwa-skill files directly*: Rejected — file-based skills do not scale to a multi-user web app and cannot support runtime composition.
- *Design a custom skill schema from scratch*: Rejected — nuwa-skill's schema is well-reasoned and gives us a head start on quality; we can map it directly to JSON.

**Mapping nuwa-skill → ElixirPill JSONB**:

| nuwa-skill section | ElixirPill.skill_schema field | Purpose |
|--------------------|-------------------------------|---------|
| 表达 DNA | `expression_dna` | Sentence length, vocabulary, rhythm, humor, certainty style |
| 核心心智模型 | `mental_models` | Named thinking frameworks with source evidence |
| 决策启发式 | `decision_heuristics` | If-X-then-Y rules |
| 价值观与反模式 | `values` + `anti_patterns` | What the skill cheries/avoids |
| 诚实边界 | `honest_limits` | Explicit limits to prevent shallow mimicry |
| 示例对话 | `example_dialogues` | Few-shot examples for the LLM |
| 身份卡 | `identity_card` | Short first-person identity statement |
| 回答工作流 | `agentic_protocol` | Optional structured workflow |

---

## Decision: Skill Fusion via Structured Merge + LLM Emergence Derivation

**Decision**: Combine multiple pills in two stages:
1. **Structured merge**: weighted blending / ordered concatenation / conflict detection on expression DNA, mental models, heuristics, and taboos.
2. **LLM emergence derivation**: one LLM call that consumes the merged structure and produces a final system prompt plus 2-3 emergent behavioral rules.

**Rationale**:
- Simple prompt concatenation causes style fragmentation (e.g., "speak like Shakespeare" + "speak like a hacker" becomes incoherent).
- Pure LLM prompt engineering without structured input is hard to test and reproduce.
- A two-stage approach gives us both determinism (structured merge) and creativity (emergence derivation).

**Alternatives considered**:
- *Concatenate all SKILL.md contents into system prompt*: Rejected — high token cost, conflicting instructions, poor emergence.
- *Use embeddings to find nearest skill and apply only one*: Rejected — violates the core "multiple pills" requirement.
- *Hand-write fusion rules for every pair of pills*: Rejected — not scalable.

**Conflict resolution default strategy**:
- Expression DNA scalar fields (e.g., formality 0-1): weighted average by pill weight.
- Categorical fields (e.g., humor_type): if conflict, list both and let emergence derivation resolve.
- Mental models / heuristics: concatenate and deduplicate by name.
- Taboos: union.
- Severe conflicts (e.g., "always short sentences" vs "always long sentences" both with high weight): mark as `inner_tension` and surface it to the user or to the emergence derivation prompt.

---

## Decision: Python Service Becomes the Language Pattern Synthesis Engine

**Decision**: Repurpose the existing Python service from a RAG engine to a **Language Pattern Synthesis Engine**. It exposes:
- `POST /api/v1/synthesis/combine` — inputs base personality + pills, returns synthesized system prompt.
- `POST /api/v1/chat/completions/stream` — inputs messages + synthesized prompt, streams LLM response.

**Rationale**:
- Python already holds the OpenAI client and LLM calling logic.
- Prompt engineering and JSON manipulation are more natural in Python than Go.
- Go remains the API gateway and business-data owner, keeping the existing architecture.

**Alternatives considered**:
- *Do synthesis in Go*: Rejected — Go is less ergonomic for complex prompt/template manipulation; keep Go focused on API/data.
- *Do synthesis in the frontend*: Rejected — API keys and LLM logic must remain server-side.

---

## Decision: Remove RAG Stack

**Decision**: Remove Qdrant, `ElixirRecipe`, vector services, document upload/extraction, and RAG retrieval from both Go and Python services.

**Rationale**:
- User explicitly stated "不做 rag 了".
- Removing Qdrant simplifies deployment and reduces resource usage.
- The new skill model does not require document ingestion.

**What to remove**:
- `backend/python/app/services/vector_service.py`
- `backend/python/app/core/vectorstore/`, `retrieval/`, `embedding/`
- `backend/go/model/models.go` → `ElixirRecipe` model
- `backend/go/service/recipe_service.go`
- `backend/go/handler/` recipe/upload handlers (need to verify handler names)
- Qdrant service in `docker-compose.yml`
- Frontend recipe/upload pages

**What to keep**:
- `DaoAgent`, `ElixirPill`, `AgentPill`, `ChatSession`, `ChatMessage`
- PostgreSQL, Go API gateway, Python LLM service, React frontend, Nginx, Docker Compose

---

## Decision: Cache Synthesized Language Pattern Per Agent

**Decision**: Cache the synthesized system prompt + emergence rules for each `DaoAgent`. Invalidate and recompute when:
- Agent's base personality changes.
- Agent's pill list changes (add, remove, weight change, sort change).
- Any consumed pill's `skill_schema` changes.

**Rationale**:
- Synthesis involves an LLM call; caching avoids paying that cost on every chat message.
- The cache can be stored in a new `language_patterns` table or as a JSONB column on `dao_agents`.

**Alternatives considered**:
- *Recompute on every request*: Rejected — unnecessary cost and latency.
- *Cache only in memory*: Rejected — would lose cache on restart; persistent cache is cheap.

---

## Decision: Validation Through Blind Tests + Structured Checks

**Decision**: Validate skill fusion quality through:
1. **Structured quality checks** (Python): verify every pill has required fields, no contradictory taboos, etc.
2. **Blind style recognition tests**: generate replies with different pill combinations and ask human or LLM judges to identify which pills were used.
3. **Emergence tests**: verify that A+B replies are distinguishable from A-only and B-only replies.

**Rationale**:
- nuwa-skill itself uses `quality_check.py` and `FIDELITY.md` blind tests. We can adapt these concepts.
- Without measurement, "chemical change" is subjective.

**Alternatives considered**:
- *No validation, rely on user feel*: Rejected — makes iteration slow and unreliable.
- *Use embedding similarity to target style*: Rejected — embedding distance is noisy for style; human/LLM judge is more reliable for v1.

---

## Open Questions Resolved

| Question | Resolution |
|----------|------------|
| nuwa-skill 具体架构是什么？ | 纯 prompt 工程产物，无运行时；SKILL.md 是核心接口。 |
| 金丹如何定义？ | 采用 nuwa-skill 结构化字段，持久化为 PostgreSQL JSONB。 |
| 多金丹如何组合？ | 结构化合并 + LLM 涌现推导两阶段。 |
| 冲突如何处理？ | 默认按权重折中；严重冲突标记为 inner_tension。 |
| 是否保留 RAG？ | 不保留，全部移除。 |
| 如何验证语言模式变化？ | 结构化质量检查 + 盲测 + 涌现测试。 |
| 是否保留用户/会话/消息模型？ | 保留，仅改造与金丹相关的字段。 |

## Remaining Product Decisions (Require User Input)

1. **冲突提示策略**：当多颗金丹严重冲突时，是强制提示用户，还是静默由 LLM 处理？
   - *Recommendation*: 默认静默处理并标记 `inner_tension`，在道人详情页用“丹性相冲”图标提示。

2. **试丹权限**：试丹是否需要保存临时组合？是否允许未登录用户使用？
   - *Recommendation*: 试丹不保存到数据库，仅在当前会话生效；登录用户才能保存为正式道人配置。

3. **金丹版本与作者**：是否支持多人共享金丹？是否需要版本控制？
   - *Recommendation*: V1 仅支持本地创建与复用，不实现多作者共享；保留 `version` 字段供未来扩展。

4. **默认金丹**：系统是否需要内置若干示例金丹？
   - *Recommendation*: 是，内置 3-5 个示例金丹（如“文言文”、“赛博朋克”、“鲁迅风”），帮助用户快速体验。

---

## Sources

- [alchaincyf/nuwa-skill (GitHub)](https://github.com/alchaincyf/nuwa-skill)
- [nuwa-skill/README_EN.md](https://github.com/alchaincyf/nuwa-skill/blob/main/README_EN.md)
- [nuwa-skill/SKILL.md](https://github.com/alchaincyf/nuwa-skill/blob/main/SKILL.md)
- [Agent Skills standard](https://agentskills.io)
- Existing Alchemy Furnace codebase: `backend/go/model/models.go`, `backend/go/service/*`, `backend/python/app/services/*`

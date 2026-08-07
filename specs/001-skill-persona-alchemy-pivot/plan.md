# Implementation Plan: 金丹化性 · Skill-Persona Alchemy Pivot

**Branch**: `001-skill-persona-alchemy-pivot` | **Date**: 2026-08-07 | **Spec**: [specs/001-skill-persona-alchemy-pivot/spec.md](specs/001-skill-persona-alchemy-pivot/spec.md)

**Input**: Feature specification from `/specs/001-skill-persona-alchemy-pivot/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

将「炼丹炉」从 RAG 知识库对话系统转型为 **金丹化性（Skill-Persona Alchemy）** 系统。核心体验不变：用户创建“道人”、炼制/服用“金丹”、与道人论道。但金丹的内涵从“文档知识库”变为“语言模式/人格特质的结构化修饰器”。

技术方案：
1. 借鉴 [nuwa-skill](https://github.com/alchaincyf/nuwa-skill) 的 SKILL.md 结构，将每颗金丹建模为包含“表达 DNA、心智模型、决策启发式、禁忌、诚实边界、示例对话”的结构化技能包。
2. 保留现有 `dao_agents`、`elixir_pills`、`agent_pills` 三张表的关系骨架，但大幅改造 `elixir_pills` 的字段，并删除 `elixir_recipes`、Qdrant、向量检索相关代码。
3. 在 Python 服务中新增 **Language Pattern Synthesis Engine（语言模式合成引擎）**：将道人的基础性格与已服用金丹按权重/顺序合并，生成统一的系统提示词，再调用 LLM 生成回复。
4. 多金丹组合通过“结构化合并 + LLM 涌现推导”实现：先按规则合并表达 DNA 与心智模型，再用一次 LLM 调用提炼出融合后的“丹性”与涌现规则，避免简单拼接。

## Technical Context

**Language/Version**: Go 1.21, Python 3.11, TypeScript/React 18

**Primary Dependencies**:
- Go: Gin, GORM, zap
- Python: FastAPI, OpenAI SDK, httpx
- Frontend: React 18, TypeScript, Tailwind CSS
- Infrastructure: PostgreSQL 14, Docker Compose, Nginx
- **Removed**: Qdrant, LangChain retrieval stack

**Storage**: PostgreSQL 14（保留业务数据），不再使用 Qdrant 向量存储。

**Testing**: Go testing + testify, Python pytest, frontend Vitest/Jest（需确认当前测试框架）。

**Target Platform**: Docker Compose 本地/服务器部署，Web 浏览器访问。

**Project Type**: Web application (frontend + backend microservices).

**Performance Goals**:
- 单次对话请求 P95 < 3s（取决于 LLM）
- 语言模式合成（非 LLM 部分）P95 < 100ms
- 支持同时在线道人数量由 PostgreSQL + Go 网关决定，目标 100+ 并发会话

**Constraints**:
- 必须兼容现有 OpenAI 兼容 API 配置
- 必须移除 Qdrant 依赖，简化部署
- 保留现有用户、会话、消息数据模型，避免大规模迁移

**Scale/Scope**:
- 单租户部署（当前无多租户需求）
- 金丹数量初期目标 < 1000，道人数量 < 1000
- 每颗金丹为结构化 JSON/文本，大小 < 50KB

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Project `.specify/memory/constitution.md` is currently a template with placeholder principles and has not been ratified. Therefore no active constitutional gates apply to this feature.

**Gate Result**: PASS (no ratified constraints; design proceeds under default engineering judgment).

## Project Structure

### Documentation (this feature)

```text
specs/001-skill-persona-alchemy-pivot/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
backend/
├── go/
│   ├── cmd/server/          # 入口
│   ├── dao/                 # GORM 数据访问（改造：移除 recipe/vector 相关）
│   ├── handler/             # HTTP/WebSocket handler（改造：移除 RAG 调用）
│   ├── middleware/          # 中间件
│   ├── model/               # 数据模型（改造：ElixirPill 改为 skill 结构）
│   ├── pkg/config/          # 配置（移除 Qdrant/PythonRAG 相关）
│   ├── pkg/response/        # 统一响应
│   ├── pkg/utils/           # 工具
│   └── service/             # 业务逻辑
│       ├── agent_service.go       # 道人 CRUD + 服用/解绑金丹
│       ├── pill_service.go        # 金丹 CRUD（从知识库改为 skill）
│       ├── chat_service.go        # 会话管理 + 调用 Python 语言合成引擎
│       └── synthesis_client.go    # 调用 Python 合成引擎的 HTTP 客户端
└── python/
    └── app/
        ├── api/             # FastAPI 路由（移除 RAG/向量路由，新增 synthesis 路由）
        ├── core/            # 核心配置
        ├── models/          # Pydantic schemas
        ├── services/
        │   ├── chat_service.py           # 流式/非流式对话（调用 LLM）
        │   └── language_synthesis_service.py  # NEW: 语言模式合成引擎
        └── main.py

frontend/
├── src/
│   ├── components/    # 复用并新增炼丹、道人、试丹组件
│   ├── contexts/      # 全局状态
│   ├── pages/         # 页面改造：移除知识库/文档上传，聚焦道人、金丹、试丹、会话
│   ├── services/      # API 调用改造
│   ├── styles/        # 道教风格 UI
│   └── utils/         # 工具
└── tests/

infra/                 # Docker、Nginx 配置（移除 Qdrant 服务）
docker-compose.yml     # 移除 qdrant 服务与依赖
```

**Structure Decision**: 保留现有的前后端 + Python 微服务架构，但重新划分职责：
- Go 作为 API 网关与业务数据持久化层，负责道人、金丹、服用关系、会话消息。
- Python 作为“语言模式合成引擎 + LLM 调用层”，负责把性格+金丹合成为系统提示词并生成回复。
- React 前端聚焦新交互：炼丹（创建 skill）、道人养成、试丹、论道。

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No constitutional violations. No complexity tracking required.

## Research Summary

See full [research.md](research.md). Key decisions:

- **nuwa-skill 是可借鉴的“技能文件格式”，不是运行时**。它使用 SKILL.md（YAML frontmatter + markdown）描述一个人的表达 DNA、心智模型、决策启发式等。它没有内置多技能融合机制——这正是本项目要填补的空白。
- **金丹数据模型采用 nuwa-skill 的结构化字段**：expression_dna、mental_models、decision_heuristics、taboos、honest_limits、example_dialogues，但存储在 PostgreSQL JSONB 中，而非文件。
- **多金丹融合策略**：先结构化合并（加权 blending、去重、冲突检测），再用一次 LLM 调用推导“丹性融合”与涌现规则，生成最终系统提示词。这样可避免简单拼接导致的风格撕裂。
- **冲突消解默认策略**：当两颗金丹在某维度（如句式长度、正式程度）冲突时，按用户配置的权重折中；若冲突严重，标记为“丹性相冲”并提示用户，或让 LLM 在回复中呈现内在张力。
- **移除 RAG**：删除 Qdrant、ElixirRecipe、向量服务、文档上传/提取流程，简化部署。

## Design Decisions (Pre-Data-Model)

- **保留 Agent-Pill 关系表**：现有 `agent_pills` 表结构基本可用，仅需增加 `weight` 和 `sort_order` 字段以支持权重与服用顺序。
- **改造 ElixirPill 表**：删除 `status`、`vector_count` 等 RAG 字段，新增 `skill_schema` JSONB 字段存储 nuwa-skill 结构化内容，以及 `version`、`author`、`tags` 等元数据。
- **新增 LanguagePattern 缓存表/字段**：为每个道人缓存当前合成后的系统提示词与涌现规则，避免每次对话重复合成。当道人基础性格或服用金丹变化时失效/重建。
- **Python 服务端新增 `/api/v1/synthesis/combine` 与 `/api/v1/chat/completions/stream`**：前者接收性格+金丹列表返回合成后的系统提示词；后者接收消息+合成提示词返回流式回复。Go 端根据缓存决定是否调用合成接口。
- **前端页面重构**：
  - 移除“丹方/文档/知识库”相关页面
  - 新增“炼丹房”页面：创建/编辑金丹（skill）
  - 改造“道人”页面：配置基础性格 + 服用金丹（可拖拽排序、调权重）
  - 新增“试丹”页面：临时组合性格+金丹，快速预览效果
  - 保留“论道”页面，但移除 RAG 引用来源展示

## Risks & Mitigations

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 多金丹融合效果不稳定 | 高 | 先做结构化合并再做 LLM 涌现推导；提供“试丹”快速迭代；定义可量化的风格一致性测试 |
| 移除 RAG 后用户失去文档问答能力 | 中 | 明确产品定位为“人格/语言风格模拟”，非知识问答；若未来需要，可后续以“记忆金丹”形式重新引入 |
| 现有数据迁移 | 低 | 保留会话消息表；旧金丹数据可保留但字段废弃，或提供一次性迁移脚本 |
| LLM 调用成本上升 | 中 | 缓存合成后的系统提示词；使用较小模型完成合成，主对话使用用户指定模型 |

## Next Steps

1. 生成 [research.md](research.md) 记录完整调研与决策依据。
2. 生成 [data-model.md](data-model.md) 定义改造后的实体与关系。
3. 生成 [contracts/](contracts/) 定义前后端与内部服务接口。
4. 生成 [quickstart.md](quickstart.md) 提供新功能本地启动与验证步骤。
5. 更新 `CLAUDE.md` 计划引用。
6. 使用 `/speckit-tasks` 生成实施任务列表。

# 系统架构文档

## 概述

「炼丹炉」现已转型为**金丹化性（Skill-Persona Alchemy）**人格塑造系统。整体为微服务架构，由四个部分组成：前端 React SPA、Go API 网关、Python 语言引擎、PostgreSQL 数据库。通过 Docker Compose 一键编排部署。

金丹是**语言模式/人格特质的结构化技能包**；系统核心是把道人的基础性格与所服金丹合成为统一的系统提示词，再调用 LLM 生成回复。

```
┌─────────────────────────────────────────────────────────────┐
│                        用户层                                │
│                  Web 浏览器 / 手机浏览器                      │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTPS
┌─────────────────────────▼───────────────────────────────────┐
│                      Nginx 网关（南天门）                     │
│         静态文件服务 / 反向代理                               │
│                    端口: 80                                  │
└──────┬──────────────────────────────┬───────────────────────┘
       │ /api/v1/*                    │ /
┌──────▼──────────────┐    ┌─────────▼──────────────────┐
│    Go API 网关       │    │   React 前端 (静态文件)      │
│    端口: 8080        │    │   端口: 80 (Nginx 提供)      │
│                     │    │                            │
│  ┌───────────────┐  │    │  - 首页                     │
│  │   REST API    │  │    │  - 道人 (Agents)            │
│  │   WebSocket   │  │    │  - 金丹 (Pills)             │
│  └───────┬───────┘  │    │  - 论道 (Chat)              │
└──────────┼──────────┘    │  - 设置                     │
           │               └────────────────────────────┘
           │ HTTP
┌──────────▼──────────────────────────────────────────┐
│           Python 语言引擎（右白虎）                   │
│           端口: 8000                                │
│                                                     │
│  ┌────────────────────┐  ┌────────────────────┐     │
│  │ 语言模式合成引擎     │  │   LLM 对话服务      │     │
│  │ (结构化合并 +        │  │  (流式/非流式,      │     │
│  │  LLM 涌现推导)      │  │   OpenAI 兼容)     │     │
│  └────────────────────┘  └────────────────────┘     │
└─────────────────────────────────────────────────────┘
           │
           │ SQL
┌──────────▼──────────┐
│    PostgreSQL       │
│    端口: 5432       │
│                    │
│  - dao_agents       │  道人
│  - elixir_pills     │  金丹（skill_schema）
│  - agent_pills      │  服用记录（weight / sort_order）
│  - language_patterns│  语言模式缓存
│  - chat_sessions    │  会话
│  - chat_messages    │  消息
└─────────────────────┘
```

## 服务说明

### 1. 前端 React SPA

- **技术栈**: React 18 + TypeScript + Tailwind CSS + shadcn/ui
- **路由**: HashRouter，兼容静态部署
- **特性**: 响应式设计，一套代码兼容 Web 端和 H5 移动端
- **页面**: 首页、道人（Agents）、道人详情（AgentDetail）、金丹（Pills）、金丹编辑（PillDetail）、论道（Chat）、设置（Settings）

### 2. Go API 网关

- **技术栈**: Go 1.21 + Gin + GORM + WebSocket + zap
- **职责**:
  - 接收前端 HTTP / WebSocket 请求
  - 业务数据管理（道人、金丹、服用记录、会话、消息）
  - 语言模式缓存（LanguagePattern）的读取与失效管理
  - 调用 Python 语言引擎（合成 / 对话）
  - 流式对话转发
- **架构模式**: 分层架构（handler → service → dao）

### 3. Python 语言引擎

- **技术栈**: FastAPI + OpenAI SDK + httpx
- **职责**:
  - **语言模式合成**：将道人基础性格与所服金丹的 skill_schema 按权重/顺序做结构化合并（加权 blending、去重、冲突检测），再用一次 LLM 调用推导融合后的"丹性"与涌现规则，生成最终系统提示词
  - **LLM 对话**：以合成后的系统提示词调用 OpenAI 兼容接口，支持流式与非流式回复
  - **试丹预览**：对临时组合执行同样的合成与对话流程，不落库

### 4. 数据存储

| 存储 | 用途 | 数据 |
|------|------|------|
| PostgreSQL | 业务数据 | 道人、金丹、服用记录、语言模式缓存、会话、消息 |

## 数据模型

| 实体 | 表 | 说明 |
|------|----|----|
| DaoAgent | `dao_agents` | 道人：基础 AI 人格（名称、性格、模型、状态） |
| ElixirPill | `elixir_pills` | 金丹：`skill_schema`(JSONB) + tags/author/version/is_builtin |
| AgentPill | `agent_pills` | 服用记录：道人与金丹的多对多关系，含 `weight`(0-10) 与 `sort_order` |
| LanguagePattern | `language_patterns` | 语言模式缓存：合成后的系统提示词、涌现规则、冲突、来源指纹 |
| ChatSession | `chat_sessions` | 对话会话 |
| ChatMessage | `chat_messages` | 对话消息（`sources` 字段废弃，保留为空） |

详细字段见 [specs/001-skill-persona-alchemy-pivot/data-model.md](../specs/001-skill-persona-alchemy-pivot/data-model.md)。

## 核心流程

### 金丹化性（语言模式合成）

```
用户请求（论道 / 试丹）
    │
    ▼
Go API:
  1. 取道人基础性格 + 已服用金丹（按 sort_order 排序，携带 weight）
  2. 计算来源指纹（personality + pills + weights 的 hash）
  3. 查询 language_patterns 缓存：
     - 指纹一致且 is_valid → 直接使用缓存的系统提示词
     - 否则 → 调用 Python 合成引擎
    │
    ▼ HTTP
Python 语言引擎:
  1. 结构化合并：按权重折中表达 DNA（句式长度、正式程度等），
     合并/去重心智模型、决策启发式、禁忌、诚实边界
  2. 冲突检测：维度严重冲突时标记为「丹性相冲」（inner_tensions）
  3. LLM 涌现推导：一次 LLM 调用提炼融合后的"丹性"与涌现规则
  4. 返回合成系统提示词 + emergence_rules + inner_tensions
    │
    ▼
Go API:
  4. 写入/更新 language_patterns 缓存
  5. 以合成提示词 + 会话消息调用 LLM（流式）
  6. 通过 WebSocket 流式转发给客户端并保存消息
```

### 缓存失效规则

以下任一变更会使道人的 LanguagePattern 缓存失效（`is_valid=false` 或删除），下次对话时重新合成：

- 道人基础性格（personality）被修改
- 服用记录变化：绑定、解绑金丹，或调整 weight / sort_order
- 任一已服用金丹的 `skill_schema` 被更新

缓存通过 `source_fingerprint`（personality + 排序后的 pills + weights 的 SHA256）判断是否命中。

## 技术选型理由

| 技术 | 选型理由 |
|------|---------|
| React + TS | 组件化开发，类型安全，生态丰富 |
| Go | 高并发，编译型，资源占用低，适合 API 网关 |
| Python + FastAPI | AI/LLM 生态成熟，适合合成引擎与模型调用 |
| PostgreSQL | 成熟稳定，JSONB 适合存储 skill_schema |
| Nginx | 高性能静态文件服务，反向代理 |
| Docker Compose | 一键编排，环境隔离，易于部署 |

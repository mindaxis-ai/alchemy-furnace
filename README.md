# 炼丹炉 &middot; Alchemy Furnace

<p align="center">
  <img src="https://img.shields.io/badge/React-18-61DAFB?logo=react" />
  <img src="https://img.shields.io/badge/Go-1.21-00ADD8?logo=go" />
  <img src="https://img.shields.io/badge/Python-3.11-3776AB?logo=python" />
  <img src="https://img.shields.io/badge/PostgreSQL-14-4169E1?logo=postgresql" />
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker" />
  <img src="https://img.shields.io/badge/License-Custom-orange.svg" />
</p>

<p align="center">
  <i>"道生一，一生二，二生三，三生万物——万物皆可炼为金丹"</i>
</p>

<p align="center">
  <a href="#简介">简介</a> &middot;
  <a href="#特性">特性</a> &middot;
  <a href="#架构">架构</a> &middot;
  <a href="#快速开始">快速开始</a> &middot;
  <a href="#概念">概念</a> &middot;
  <a href="#API">API</a> &middot;
  <a href="#文档">文档</a>
</p>

---

## 简介

**炼丹炉**（Alchemy Furnace）是一款以道教炼丹文化为设计灵感的**金丹化性（Skill-Persona Alchemy）**人格塑造系统。在这里，语言模式与人格特质化作**金丹**，AI Agent 化作**道人**——道人服用金丹后，其言谈举止、思维方式便被金丹的"丹性"所化，以焕然一新的语言风格与你论道对答。

> 古人云："丹成而龙虎伏，道备而鬼神惊。"今人以 AI 为火，以语言模式为药，炼就人格之金丹，养出性情各异之道人。

## 关于作者

本项目「炼丹炉 (Alchemy Furnace)」由 **[yusanwen-code](https://github.com/yusanwen-code)** 开发维护。

- **GitHub**: [https://github.com/yusanwen-code](https://github.com/yusanwen-code)
- **仓库地址**: [https://github.com/yusanwen-code/alchemy-furnace](https://github.com/yusanwen-code/alchemy-furnace)
- **开源方式**: 私人开源，商业闭源。个人使用、学习研究可自由使用；商业使用需单独授权，且必须保留项目名称和开发者信息。

## 特性

### 核心能力

- **炼丹入炉** - 以结构化技能包（skill schema）炼制金丹：表达 DNA、心智模型、决策启发式、禁忌、诚实边界、示例对话，一应俱全
- **道人养成** - 创建个性化 AI Agent（道人），配置基础性格、选用大模型
- **服用金丹** - 一个道人可服用多颗金丹，支持权重与服用顺序调节
- **金丹化性** - 语言模式合成引擎将道人基础性格与所服金丹结构化合并，再经 LLM 涌现推导，生成统一的"丹性"系统提示词
- **多丹融合** - 多颗金丹按权重折中融合，冲突维度自动检测（丹性相冲），涌现规则提炼，避免风格撕裂
- **试丹预览** - 临时组合性格与金丹，不绑定道人即可快速预览合成效果
- **论道对答** - 流式对话，道人始终以融合后的语言风格回应

### 技术亮点

- **响应式设计** - 一套 React 代码同时兼容桌面 Web 端和手机 H5 端
- **微服务架构** - Go 负责 API 网关与业务数据，Python 负责语言模式合成引擎与 LLM 调用，职责分明
- **流式对话** - WebSocket 实时传输，打字机效果呈现 AI 回复
- **合成缓存** - 道人当前的合成系统提示词按需缓存，性格或金丹变化时自动失效重建
- **道教美学** - 朱砂红、金箔黄、宣纸米白，CSS 动画炼丹特效，沉浸式体验

## 架构

```
                    紫府（前端 React）
                         │
                    南天门（Nginx）
                    /          \
              左青龙        右白虎
           （Go API）   （Python 语言引擎）
              │              │
         玉简库（PG）    LLM（OpenAI 兼容）
```

| 服务 | 技术 | 职责 | 端口 |
|------|------|------|------|
| **紫府** | React 18 + TypeScript + Tailwind CSS | 用户界面 | 80 (Nginx) |
| **左青龙** | Go 1.21 + Gin + GORM | API 网关、业务逻辑、道人/金丹/会话持久化 | 8080 |
| **右白虎** | Python 3.11 + FastAPI | 语言模式合成引擎、LLM 调用 | 8000 |
| **玉简库** | PostgreSQL 14 | 业务数据持久化 | 5432 |
| **南天门** | Nginx | 反向代理、静态文件 | 80 |

## 快速开始

### 环境要求

- [Docker](https://docs.docker.com/get-docker/) 20.10+
- [Docker Compose](https://docs.docker.com/compose/install/) 2.0+
- 一个 OpenAI 兼容的 API Key（支持 OpenAI、DeepSeek、通义千问等）

### 一键部署

```bash
# 1. 克隆仙府
git clone https://github.com/yusanwen-code/alchemy-furnace.git
cd alchemy-furnace

# 2. 配置天机
cp .env.example .env
# 编辑 .env，填入你的 OPENAI_API_KEY

# 3. 点火烧炉
make deploy

# 4. 开炉取丹
# 打开浏览器访问 http://localhost
```

### 使用 Make 命令

```bash
make help       # 查看所有可用命令
make init       # 初始化环境
make build      # 构建镜像
make up         # 启动服务
make down       # 停止服务
make logs       # 查看日志
make ps         # 查看状态
make clean      # 清理所有数据
make dev-front  # 前端开发模式
make dev-go     # Go 后端开发模式
make dev-python # Python 语言引擎开发模式
```

### 本地开发（无 Docker）

需要本机具备：PostgreSQL 14+（如 [Postgres.app](https://postgresapp.com/)）、Go 1.21+、Python 3.11+、Node.js 20+。

```bash
# 1. 准备数据库（本地 PostgreSQL 监听 5432）
createuser alchemy && createdb -O alchemy alchemy_db
psql -c "ALTER USER alchemy PASSWORD 'alchemy123';"

# 2. 配置环境（DB_HOST 改为 localhost，PYTHON_ENGINE_BASE_URL 改为 http://localhost:8000）
cp .env.example .env  # 按本机地址编辑

# 3. 分三个终端启动
make dev-python   # Python 语言引擎 → http://localhost:8000
make dev-go       # Go API 网关（自动迁移 + 写入内置金丹）→ http://localhost:8080
make dev-front    # React 前端 → http://localhost:3000
```

### 首次使用指南

1. **炼制金丹** - 进入「炼丹房」，创建金丹：填写表达 DNA（句式长度、正式程度、常用词汇、禁忌词）、心智模型、决策启发式、示例对话等结构化内容
2. **孕育道人** - 进入「道人府」，点击「收徒」，配置道人的名称、基础性格和选用的大模型
3. **服用金丹** - 进入道人详情页，选择已炼制好的金丹让道人服用，可调节权重与服用顺序
4. **试丹预览**（可选）- 进入「试丹」页面临时组合性格与金丹，快速预览合成效果
5. **开炉论道** - 进入「论道」，选择道人，开始对话！

## 概念

### 金丹（Elixir Pill）

> "金丹者，混元一气之所结也。"

金丹即**语言模式/人格特质的结构化技能包**。每颗金丹包含：

| 字段 | 说明 |
|------|------|
| identity_card | 身份卡：金丹所化人格的自我认知 |
| expression_dna | 表达 DNA：句式长度、正式程度、常用词汇、禁忌词 |
| mental_models | 心智模型：看待问题的思维框架 |
| decision_heuristics | 决策启发式：特定情境下的表达策略 |
| values / anti_patterns | 价值取向与反模式 |
| honest_limits | 诚实边界：能力局限的坦诚声明 |
| example_dialogues | 示例对话：风格的具体示范 |

### 道人（Dao Agent）

> "道者，万物之所系也。"

道人即 **AI Agent**。每个道人有独立的基础性格、系统提示词和选用的大模型。道人的言谈由"基础性格 + 所服金丹"共同塑造。

### 服用金丹（Bind Pill）

> "服食金丹，脱胎换骨，通达古今。"

将金丹与道人绑定，可配置**权重**（weight）与**服用顺序**（sort_order）。一个道人可服用多颗金丹，众丹之性融于一身。

### 金丹化性（Language Pattern Synthesis）

> "采药 → 配比 → 入炉 → 温养 → 开炉取丹"

对应合成流程：

| 炼丹步骤 | 合成步骤 | 说明 |
|---------|---------|------|
| 采药 | 取性 | 读取道人基础性格与所服金丹 |
| 配比 | 结构化合并 | 按权重 blending 表达 DNA 与心智模型，去重、检测冲突 |
| 入炉 | 涌现推导 | 一次 LLM 调用提炼融合后的"丹性"与涌现规则 |
| 温养 | 缓存 | 合成后的系统提示词缓存于道人，变化时自动重建 |
| 开炉取丹 | 论道 | 以合成提示词调用 LLM 生成回复 |

## 截图

<p align="center">
  <i>🖼️ 炼丹房 - 金丹（技能包）管理</i>
</p>

<p align="center">
  <i>🖼️ 道人府 - AI Agent 管理</i>
</p>

<p align="center">
  <i>🖼️ 论道 - 流式对话</i>
</p>

## API

详见 [docs/api.md](docs/api.md)

### 快速示例

```bash
# 创建金丹（结构化技能包）
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
        "vocabulary": ["之", "乎", "者", "也"],
        "taboo_words": ["你", "我", "的", "了"]
      }
    },
    "tags": ["文言文", "古雅"],
    "author": "system",
    "version": "1.0.0"
  }'

# 创建道人
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "太上老君",
    "personality": "你是太上老君，道家的始祖。言谈充满无上智慧，善用《道德经》的哲理来阐释万物。",
    "model_name": "gpt-4o"
  }'

# 服用金丹（支持权重与顺序）
curl -X POST http://localhost:8080/api/v1/agents/1/pills \
  -H "Content-Type: application/json" \
  -d '{"pill_id": 1, "weight": 1.0, "sort_order": 0}'

# 创建会话并对话（WebSocket）
# 详见 API 文档与 specs/001-skill-persona-alchemy-pivot/quickstart.md
```

## 文档

| 文档 | 说明 |
|------|------|
| [docs/architecture.md](docs/architecture.md) | 系统架构详解 |
| [docs/api.md](docs/api.md) | API 接口文档 |
| [docs/frontend.md](docs/frontend.md) | 前端开发文档 |
| [docs/deployment.md](docs/deployment.md) | 部署指南 |
| [specs/001-skill-persona-alchemy-pivot/quickstart.md](specs/001-skill-persona-alchemy-pivot/quickstart.md) | 金丹化性系统快速验证指南 |

## 项目结构

```
alchemy-furnace/
├── README.md                    # 项目说明
├── docker-compose.yml           # Docker 编排
├── .env.example                 # 环境变量模板
├── Makefile                     # 常用命令
├── docs/                        # 维护文档
│   ├── architecture.md          # 架构文档
│   ├── api.md                   # API 文档
│   ├── frontend.md              # 前端文档
│   └── deployment.md            # 部署指南
├── backend/
│   ├── go/                      # Go API 网关
│   │   ├── cmd/server/
│   │   ├── dao/
│   │   ├── handler/
│   │   ├── model/
│   │   ├── pkg/
│   │   ├── service/
│   │   ├── go.mod
│   │   └── Dockerfile
│   └── python/                  # Python 语言模式合成引擎
│       ├── app/
│       │   ├── api/
│       │   ├── core/
│       │   ├── models/
│       │   └── services/
│       ├── requirements.txt
│       └── Dockerfile
└── frontend/                    # React 前端
    ├── src/
    ├── public/
    ├── package.json
    └── Dockerfile
```

## 技术栈

### 前端
- React 18 + TypeScript
- Tailwind CSS + shadcn/ui
- Vite
- HashRouter（兼容静态部署）
- WebSocket 客户端
- React Context API 状态管理

### 后端
- Go 1.21 + Gin
- GORM + PostgreSQL
- WebSocket
- RESTful API
- Zap 日志

### 语言模式合成引擎
- Python 3.11 + FastAPI
- OpenAI SDK（兼容任意 OpenAI 接口）
- 结构化合并 + LLM 涌现推导

## 开发计划

- [x] 金丹管理（技能包 CRUD）
- [x] 道人管理（Agent CRUD）
- [x] 服用金丹（权重与顺序配置）
- [x] 语言模式合成引擎（结构化合并 + LLM 涌现推导）
- [x] 试丹预览（临时组合快速验证）
- [x] 流式对话（WebSocket）
- [ ] 多轮对话上下文优化
- [ ] 丹性相冲可视化提示
- [ ] 多用户权限管理
- [ ] 炼丹过程可视化
- [ ] 金丹分享与 marketplace

## 许可证

本项目采用自定义许可证，详情参见 [LICENSE](LICENSE) 文件。

**简要说明：**

- **个人使用、学习研究**：可自由使用、修改、分发
- **商业使用**：必须保留项目名称「炼丹炉 (Alchemy Furnace)」和开发者信息「yusanwen-code」，且需联系作者获取商业授权

---

<p align="center">
  <i>道法自然 · 炼丹铸智</i>
</p>

<p align="center">
  Made with ☯️ by <a href="https://github.com/yusanwen-code">yusanwen-code</a>
</p>

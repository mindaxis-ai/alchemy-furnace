# 炼丹炉 · Alchemy Furnace

[English](./README_EN.md) · 中文

> **以火为引，以药为基。** 观灵气流转、察丹药自成——万物皆可炼为金丹。

炼丹炉是一个面向 AI Agent 的"语言模式工程"实验场。在这里，**金丹**是把人格特质和表达方式封装好的结构化技能包，**道人**是服用金丹的 AI Agent——一旦服用，道人的言谈举止就被这颗金丹的"丹性"所化，呈现焕然一新的语言风格。

底层是 Go 网关 + Python 合成引擎 + Next.js 前端的三段式架构，支持 OpenAI 兼容的多家模型供应商。

---

## 特性

**核心能力**
- **炼丹房** —— 结构化技能包编辑器：表达 DNA、心智模型、决策启发式、禁忌词、示例对话一应俱全
- **道人府** —— 创建并管理多个 AI Agent，各自绑定大模型
- **服用金丹** —— 一位道人可服用多颗金丹，支持权重与服用顺序
- **金丹化性** —— 合成引擎把多颗金丹按权重合并，去重、检测冲突，LLM 涌现提炼统一丹性
- **试丹预览** —— 不绑定道人，临时组合性格与金丹即可预览合成效果
- **金丹融合** —— 任意 N 枚金丹（N ≥ 2）投入融合炉，随机抽取变异算子（Promptbreeder 风格），LLM 自由发挥炼出新丹；血统可追溯，预览满意才入库
- **论道对答** —— SSE 流式对话，合成后的系统提示词全程生效

**技术亮点**
- **三段式架构** —— Go API 网关 + Python 合成引擎 + Next.js 前端，职责清晰
- **响应式设计** —— 同一套代码兼容桌面 Web 与手机 H5
- **模型协议化** —— 预置国内外常见 OpenAI 兼容供应商（DeepSeek / 通义千问 / 智谱 GLM / Kimi / 百川 / 文心一言 / Ollama），API Key 加密存储
- **合成缓存** —— 道人当前合成提示词按需缓存，性格或金丹变化时自动重建
- **演示模式** —— `DEMO_MODE=true` 一键开启内存 mock，无须 PostgreSQL 即可演示

---

## 架构

```
             浏览器 / H5
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

| 服务 | 技术栈 | 职责 | 端口 |
|------|--------|------|------|
| Frontend | Next.js 16 + React 19 + Tailwind 4 | 用户界面（中文/英文/响应式） | 3000 |
| Go API | Go 1.21+ + Gin + GORM | 业务网关、道人/金丹/会话持久化 | 8080 |
| Python Engine | Python 3.11+ + FastAPI | 语言模式合成、LLM 调用 | 8000 |
| PostgreSQL | 14+ | 业务数据 | 5432 |
| Nginx | — | 反向代理、静态文件 | 80 |

---

## 快速开始

### 一、Docker 一键部署

```bash
# 1. 克隆仓库
git clone https://github.com/yusanwen-code/alchemy-furnace.git
cd alchemy-furnace

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env：填入 OPENAI_API_KEY 与 MODEL_KEY_SECRET
# MODEL_KEY_SECRET 用于加密存储的模型 API Key，可用 openssl rand -hex 32 生成

# 3. 启动
make deploy         # 等价于 make init && make build && make up
make ps             # 查看容器状态
make logs           # 查看日志
```

浏览器打开 http://localhost 即可。

### 二、本地开发（无 Docker）

本机需要：PostgreSQL 14+（推荐 [Postgres.app](https://postgresapp.com/)）、Go 1.21+、Python 3.11+、Node.js 20+。

```bash
# 1. 准备数据库
createuser alchemy && createdb -O alchemy alchemy_db
psql -c "ALTER USER alchemy PASSWORD 'alchemy123';"

# 2. 配置 .env
cp .env.example .env
# 把 DB_HOST 改为 localhost，PYTHON_ENGINE_BASE_URL 改为 http://localhost:8000

# 3. 分三个终端启动
make dev-python     # Python 合成引擎 → http://localhost:8000
make dev-go         # Go API 网关（自动建表 + 写入内置金丹）→ http://localhost:8080
make dev-front      # Next.js 前端 → http://localhost:3000
```

也可以用 `bash scripts/dev.sh` 一键拉起三个服务（自动处理 Docker postgres 与 Postgres.app 的端口冲突）。

### 三、演示模式

演示模式**不需要 PostgreSQL**，Go 与 Python 全部走内存 mock——适合服务器做产品演示或离线体验。

```bash
# 本地开发
export DEMO_MODE=true
bash scripts/dev.sh

# 服务器一键部署
sudo bash scripts/deploy-demo.sh
```

启动时自动载入 9 颗金丹 / 9 位道人 / 9 个供应商 / 9 个模型 / 9 条会话；对话与合成由 mock Provider 返回固定中文回复。详细见 [docs/operations/deploy.md](docs/operations/deploy.md)。

---

## 首次使用

1. **炼制金丹** —— 进入「炼丹房」创建金丹：填写表达 DNA（句式长度、正式程度、常用词、禁忌词）、心智模型、示例对话
2. **孕育道人** —— 进入「道人府」收徒，配置基础性格与选用的大模型
3. **服用金丹** —— 打开道人详情页，选择金丹让道人服用，调整权重与顺序
4. **试丹预览** —— 进入「试丹」页临时组合性格与金丹，无需绑定道人即可预览效果
5. **开炉论道** —— 进入「论道」选一位道人开始对话，SSE 流式输出

---

## 核心概念

### 金丹（Elixir Pill）

结构化技能包，定义一种语言风格与人格。每颗金丹包含：

| 字段 | 说明 |
|------|------|
| `identity_card` | 身份卡：金丹所化人格的自我认知 |
| `expression_dna` | 表达 DNA：句式长度、正式程度、常用词、禁忌词 |
| `mental_models` | 心智模型：看待问题的思维框架 |
| `decision_heuristics` | 决策启发式：特定情境下的表达策略 |
| `values` / `anti_patterns` | 价值取向与反模式 |
| `honest_limits` | 诚实边界：能力局限的坦诚声明 |
| `example_dialogues` | 示例对话：风格的具体示范 |

### 道人（Dao Agent）

AI Agent 实体。拥有基础性格、选用的模型、被授予的金丹。其回复风格由"基础性格 + 合成后的丹性"统一塑造。

### 服用（Bind Pill）

把金丹与道人绑定，配置**权重**（weight）与**服用顺序**（sort_order）。一位道人可同时服用多颗金丹。

### 化性（Language Pattern Synthesis）

把多颗金丹按权重合并、去重、检测冲突，再由 LLM 涌现提炼成统一的系统提示词。过程对应炼丹五步：

| 炼丹 | 合成 | 说明 |
|------|------|------|
| 采药 | 取性 | 读取道人基础性格与所服金丹 |
| 配比 | 结构化合并 | 按权重 blending，去重、检测冲突 |
| 入炉 | 涌现推导 | LLM 提炼融合后的丹性与涌现规则 |
| 温养 | 缓存 | 合成提示词缓存于道人，变化时自动重建 |
| 开炉取丹 | 论道 | 以合成提示词调用 LLM 流式回复 |

### 融合（Pill Fusion）

任意 N 枚金丹（N ≥ 2）投入融合炉，由大模型自由发挥炼成一枚全新金丹。

**流程**：选丹 → 开炉（动画）→ 预览（含算子与血统）→ 换一炉 / 编辑 / 保存入库。

**随机性来自**每次随机抽取 7 个融合算子之一（temperature 1.0）：

| 算子 | 效果 |
|---|---|
| 夸张突变 | 特质推向极端，风格浓度翻倍 |
| 蒸馏提炼 | 只留最深层共同点 |
| 对立调和 | 矛盾成为新人格的张力引擎 |
| 角色反转 | 反转核心立场，熟悉的陌生人 |
| 血统稀释 | 一丹主导，其余点缀 |
| 基因重组 | 字段级杂交 |
| 涌现变异 | 产出原料们的「下一代」 |

新金丹的 `skill_schema.fusion_lineage` 记录父代、算子与时间，详情页可溯血统。

**技术参考**：[Promptbreeder](https://arxiv.org/abs/2309.16797)（LLM 作为变异算子执行器）、[Blended Skill Talk](https://arxiv.org/abs/2004.08449)（多技能融合为单一人格）、[EvoPrompt](https://arxiv.org/abs/2309.08532)（LLM 驱动的 prompt crossover）。

---

## API 速查

```bash
# 创建金丹
curl -X POST http://localhost:8080/api/v1/pills \
  -H "Content-Type: application/json" \
  -d '{"name": "文言文金丹", "description": "令道人开口便是之乎者也", "skill_schema": {...}, "tags": ["文言文"], "author": "system", "version": "1.0.0"}'

# 创建道人
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "太上老君", "personality": "道家始祖，言谈充满无上智慧", "model_name": "gpt-4o"}'

# 服用金丹
curl -X POST http://localhost:8080/api/v1/agents/{agent_uuid}/pills \
  -H "Content-Type: application/json" \
  -d '{"pill_id": "{pill_uuid}", "weight": 1.0, "sort_order": 0}'

# SSE 流式对话
curl -N -X POST http://localhost:8080/api/v1/chat/sse/{session_uuid} \
  -H "Content-Type: application/json" \
  -d '{"content": "何为道"}'

# 金丹融合预览（N ≥ 2 枚金丹 → 新丹，不落库）
curl -X POST http://localhost:8080/api/v1/fusion/fuse \
  -H "Content-Type: application/json" \
  -d '{"pill_uuids": ["<uuid1>", "<uuid2>"]}'
```

完整接口见 [docs/api.md](docs/api.md)。

---

## 文档

| 文档 | 说明 |
|------|------|
| [docs/architecture.md](docs/architecture.md) | 系统架构详解 |
| [docs/api.md](docs/api.md) | 完整 API 接口文档 |
| [docs/frontend.md](docs/frontend.md) | 前端开发指南 |
| [docs/operations/deploy.md](docs/operations/deploy.md) | 演示模式与服务器部署 |

---

## 贡献者必读（Pre-commit Hook）

仓库自带 **pre-commit 钩子**，在 `git commit` 时自动跑两层扫描：

- **gitleaks** —— 拦截常见 secret（API key / 私钥 / 账号凭据等 100+ 模式）
- **sensitive-word** —— 拦截本项目 specific 敏感字串（账号 ID、registry 域名）

### 安装（clone 后一次）

```bash
# 1. 装 gitleaks（macOS）
brew install gitleaks

# 2. 让 git 用本仓库的 hooks 目录
git config core.hooksPath .githooks
```

跳过机制（应急用，**不推荐**）：

```bash
SKIP_GITLEAKS=1 git commit -m "..."   # 只跳过 gitleaks
SKIP_SENSITIVE=1 git commit -m "..."  # 只跳过 sensitive-word
SKIP_PRE_COMMIT=1 git commit -m "..." # 跳过整个钩子
```

---

## 项目结构

```
alchemy-furnace/
├── README.md                    # 本文件
├── README_EN.md                 # English
├── Makefile                     # 常用命令
├── docker-compose.yml
├── .env.example
├── docs/                        # 维护文档
├── backend/
│   ├── go/                      # Go API 网关（Gin + GORM）
│   │   ├── cmd/                 # 入口
│   │   ├── internal/            # handler / service / dao
│   │   └── Dockerfile
│   └── python/                  # Python 合成引擎（FastAPI）
│       ├── app/
│       │   ├── api/             # 路由
│       │   ├── core/            # 配置 / 运行时
│       │   ├── models/          # 数据模型
│       │   └── services/        # 合成 / provider
│       └── Dockerfile
└── frontend/                    # Next.js 前端
    ├── app/                     # App Router（[locale] + (main)）
    ├── components/              # 业务组件
    ├── contexts/                # React Context
    ├── lib/                     # 工具
    ├── messages/                # i18n 字典
    ├── public/                  # 静态资源
    └── Dockerfile
```

---

## 技术栈

**前端** · Next.js 16 · React 19 · Tailwind 4 · next-intl · pnpm
**网关** · Go 1.21+ · Gin · GORM
**引擎** · Python 3.11+ · FastAPI · OpenAI SDK
**存储** · PostgreSQL 14+
**部署** · Docker Compose · Nginx

---

## 许可证

本项目采用自定义许可证，详见 [LICENSE](LICENSE)。

**简要**
- 个人使用、学习研究：可自由使用、修改、分发
- 商业使用：必须保留项目名称「炼丹炉 (Alchemy Furnace)」与开发者署名「yusanwen-code」，并联系作者获取商业授权

---

<p align="center"><i>道法自然 · 炼丹铸智</i></p>
<p align="center">Made with ☯️ by <a href="https://github.com/yusanwen-code">yusanwen-code</a></p>

# Claude Code 执行指令：LangGraph 对话编排迁移

## 首次执行：只做 Task 1

复制下面整段给 Claude Code：

```text
你现在要在 alchemy-furnace 仓库中执行 LangGraph 对话编排迁移。

仓库绝对路径：
/Users/yaoyuliang/ai_coding/alchemy-furnace

已确认的架构设计：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/specs/2026-09-02-langgraph-conversation-orchestration-design.md

唯一实施计划：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/plans/2026-09-02-langgraph-conversation-orchestration.md

架构设计提交：
604fa49 docs: design LangGraph conversation orchestration

最终目标：
将全部单聊和群聊编排迁移到 Python LangGraph。Go 只保留鉴权、业务数据 CRUD、供应商与加密凭证、正式消息和记忆持久化，以及内部事件到前端 SSE 的转换。显式 @、@全体成员、报数、停止和继续必须由确定性节点处理；开放讨论才调用默认模型作为无人格 Supervisor。不得引入完整 LangChain Agent、AutoGen、RAG 或实际工具。

本轮只能执行 Task 1：Pin LangGraph and provider dependencies。不得读取或实现 Task 2～Task 15。

执行规则：

1. 先进入仓库并确认当前分支与状态：
   cd /Users/yaoyuliang/ai_coding/alchemy-furnace
   git branch --show-current
   git status --short

2. 工作区当前可能存在 Prompt Debug 和用户自己的未提交修改。把开始时的 git status 保存为基线：
   - 不得覆盖、删除、格式化、暂存或提交任何基线文件；
   - 本轮只允许修改 Task 1 的两个文件；
   - 如果 Task 1 文件在基线中已经是 dirty，立即停止并报告冲突，不得继续；
   - `backend/go/internal/webui/out/404.html`、`backend/go/internal/webui/out/index.html`、`mask.png`、`docs/claude-prompts/` 和 `evals/` 视为用户资产或已有工作，不得清理或提交。

3. 开始前只读取：
   - 仓库根目录 AGENTS.md；
   - 架构设计的 Purpose、Goals and Non-goals、Ownership Boundary；
   - 实施计划的 Goal、Architecture、Global Constraints；
   - 实施计划中的 Task 1；
   - `backend/python/requirements.txt`；
   - `scripts/build-python-runtime.sh` 中 Python 版本相关段落；
   - `backend/python/app/tests/conftest.py`。
   不要一次加载整个实施计划或全仓库。

4. 严格按 Task 1 的 checkbox 顺序执行 TDD：
   - 先创建导入冒烟测试；
   - 运行精确测试并确认失败确实来自缺少 LangGraph/adapter；
   - 按计划写入固定依赖版本并安装到现有 `.venv`；
   - 运行 Task 1 指定测试；
   - 检查依赖解析结果，不得自行换版本或改用未批准框架。

5. 只能修改：
   - backend/python/requirements.txt
   - backend/python/app/tests/test_langgraph_dependencies.py

6. 禁止事项：
   - 不得开始写 Graph、ModelGateway、API 或 Go 代码；
   - 不得修改现有 OpenAI 调用实现；
   - 不得运行全仓库格式化；
   - 不得删除、跳过或放宽测试；
   - 不得 push、合并 master、打 tag、改版本号、打包或发版；
   - 不得在日志、测试输出或提交中加入 API Key。

7. 如果固定依赖无法解析或现有 Python 测试因依赖升级出现真实回归：
   - 停止修改；
   - 保留失败证据；
   - 报告冲突包、解析器输出、受影响测试和最小修订建议；
   - 等待用户确认，不得自行替换版本。

8. 当前 Task 验证通过后，只暂存 Task 1 的两个文件，先运行：
   git diff --cached --check
   git diff --cached --name-only
   确认无基线文件后，使用计划规定的提交信息提交。不要 push。

9. Task 1 完成后立即停止，等待用户明确要求继续 Task 2。

完成后必须按以下格式回复：

Task：Task 1 — Pin LangGraph and provider dependencies
状态：完成 / 阻塞
提交：<commit hash；没有提交写“未提交”>
修改文件：
- <路径>
失败测试验证：
- <命令>
- <退出码与预期失败原因>
通过测试验证：
- <命令>
- <退出码与准确结果>
依赖解析：
- <Python 版本与关键包版本>
未解决问题：
- <没有则写“无”>
工作区保护：
- <列出确认未被触碰的基线文件>
下一步：等待用户确认后执行 Task 2

现在只执行 Task 1。
```

## 后续任务：一次只执行一个 Task

Task 1 完成并经检查后，每次复制下面内容并替换 `N`：

```text
继续执行 LangGraph 对话编排实施计划中的 Task N，只做 Task N，不得执行 Task N+1。

仓库：
/Users/yaoyuliang/ai_coding/alchemy-furnace

架构设计：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/specs/2026-09-02-langgraph-conversation-orchestration-design.md

实施计划：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/plans/2026-09-02-langgraph-conversation-orchestration.md

先执行 `git branch --show-current` 和 `git status --short`，记录工作区基线。只读取设计中与当前 Task 相关的章节、计划头部 Global Constraints、Task N 全文、Task N 的 Files 和 Interfaces 明确依赖的前置定义。不要把后续 Task 或全仓库一次加载进上下文。

严格执行当前 Task 的 checkbox：先写失败测试并确认失败原因，再做最小实现，运行精确测试与指定包测试，最后只暂存当前 Task 文件并执行 `git diff --cached --check` 和 `git diff --cached --name-only`。使用计划规定的 Conventional Commit 信息提交，但不要 push。

所有开始时已 dirty 的文件都属于用户。若当前 Task 必须修改其中任何文件，先停止并报告“冲突文件、当前差异、Task 需要修改的原因”，等待用户先处理或明确授权；不得覆盖、混合提交或擅自还原。不得用 `git reset --hard`、`git checkout --`、`git clean` 或递归删除命令。

不得顺手重构，不得实现后续 Task，不得更改已确认的 Go/Python 所有权边界，不得引入完整 LangChain Agent、AutoGen、RAG 或真实工具。不得记录或输出密钥。遇到无关基线失败只报告，不修复。

完成后按以下格式汇报并停止：

Task：Task N — <名称>
状态：完成 / 阻塞
提交：<hash 或“未提交”>
修改文件：
- <路径>
失败测试验证：
- <命令、退出码、预期失败原因>
通过测试验证：
- <命令、退出码、准确结果>
架构约束检查：
- <本 Task 如何保持 Go CRUD / Python LangGraph 边界>
安全检查：
- <凭证是否进入 state、checkpoint、event、log；应为否>
未解决问题：
- <没有则写“无”>
工作区保护：
- <确认基线文件未被误改或误提交>
下一步：等待用户确认后执行 Task N+1
```

## 单 Task 代码评审

```text
只评审刚完成的 LangGraph 迁移 Task N，不修改代码、不提交、不 push。

架构设计：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/specs/2026-09-02-langgraph-conversation-orchestration-design.md

实施计划：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/plans/2026-09-02-langgraph-conversation-orchestration.md

检查当前 Task 的提交、Files、Interfaces、失败测试证据和通过测试证据。重点检查：
- 是否提前实现了后续 Task；
- Go 是否重新承担了意图、选人、提示词或模型编排；
- credentials 是否进入 graph state、SQLite checkpoint、event、debug 或日志；
- 是否把显式报数错误地交给 Supervisor；
- 是否真正做到 assistant_final 幂等落库、partial delta 不落正式消息；
- 是否覆盖、还原或提交了任务开始时已有的用户改动；
- 是否删除测试、放宽断言或用提高 token/重试上限掩盖错误。

按 P0/P1/P2 输出问题，每项必须给出文件路径和行号，并说明违反的设计或 Task 条款。没有问题时明确写“Task N 可以进入下一任务”。
```

## 全部 Task 完成后的最终验收

```text
LangGraph 对话编排 Task 1～Task 15 已声称完成。现在只做最终验收，不新增功能、不修改代码、不提交、不 push、不发版。

设计：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/specs/2026-09-02-langgraph-conversation-orchestration-design.md

实施计划：
/Users/yaoyuliang/ai_coding/alchemy-furnace/docs/superpowers/plans/2026-09-02-langgraph-conversation-orchestration.md

逐项核对 Definition of Done 和每个 Task 的提交范围。运行计划规定的 Python、Go、前端、Python runtime assembly 和 Wails 桌面验证。浏览器开发模式不能替代 Wails 验收。

必须真实执行核心用例：成员依次为张雪峰、李雪琴、贾玲、沈腾，用户发送“@全体成员 全体都有！报数！”。验证四人严格输出 1、2、3、4；无 Supervisor 调用、无漏人、无重复、无考研建议。另验证开放讨论调用默认 Supervisor、Supervisor 失败回退、单人模型失败不阻断其他人、Prompt Debug 开关与脱敏、停止后 partial 不落库、继续不重复已完成道人、重启后 checkpoint 恢复、未知 OpenAI-compatible 供应商兜底。

列出每条命令、退出码、通过数量、失败测试、桌面人工步骤和结果。将无关基线失败与本次回归分开。最终输出：通过项、失败项、阻塞项、安全检查、遗留风险和是否具备合并条件。
```

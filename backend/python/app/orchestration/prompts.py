"""Daoist 提示词编译：把机械任务、记忆与历史消息编译为 LangChain 消息列表。

规则（设计文档 §5/§10 落地）：
- 机械任务约束（报数编号、点名任务）以原文进入系统提示，位置高于记忆与闲聊；
  任务文本由计划方逐字生成，编译层不做改写（原样保留 = 可断言）。
- 记忆与历史均为 Go 权威快照的只读视图；编译层只做裁剪与格式注入，不回写。
- 编译只消费纯数据快照，不触碰凭据；凭据永不进入消息文本。
"""

from __future__ import annotations

import json

from langchain_core.messages import AIMessage, BaseMessage, HumanMessage, SystemMessage

from app.orchestration.contracts import (
    AgentSnapshot,
    MemorySnapshot,
    MessageSnapshot,
    ResponseBudget,
    SemanticUnderstanding,
    UserTurnSnapshot,
)

#: 历史窗口：只保留最近 N 条（含当前用户轮之前的全部发言）。
HISTORY_WINDOW = 20
#: 单条历史消息字符预算：超长截尾，防单条巨文打爆窗口。
MAX_MESSAGE_CHARS = 2000
#: 单条记忆字符预算：与 select_memories 的裁剪同源（兜底常量）。
MAX_MEMORY_CHARS = 300
#: 单次发言最多注入的记忆条数（与 select_memories 一致）。
MAX_MEMORIES = 8


def _trim(text: str, limit: int) -> str:
    """超长文本截尾；边界处留占位符避免与正文混淆。"""
    if len(text) <= limit:
        return text
    return text[: limit - 1] + "…"


def compile_messages(
    agent: AgentSnapshot,
    user_turn: UserTurnSnapshot,
    history_snapshot: list[MessageSnapshot],
    selected_memories: list[MemorySnapshot],
    understanding: SemanticUnderstanding,
    budget: ResponseBudget,
    task: str | None = None,
) -> list[BaseMessage]:
    """编译一次 Daoist 调用的完整消息列表。

    :param agent: 当前发言道人（system_prompt 已含人设与启用金丹效果）。
    :param user_turn: 当前用户轮（永远作为最后一条 user 消息）。
    :param history_snapshot: 权威历史（取最近窗口，逐条截尾）。
    :param selected_memories: 已筛选的当前道人记忆（原样注入，不做改写）。
    :param task: 机械任务文本（可空 = 常规回应）；原样进入系统提示。
    :return: [system, *history, user_turn] 的消息列表。
    """
    composed_prompt = agent.system_prompt.strip()
    system_lines: list[str] = []
    if task:
        system_lines.append(
            f"【回合任务】{task}\n机械任务优先于人物风格、示例对白和一般回答偏好。"
        )
    system_lines.append(
        composed_prompt
        or f"【身份与性格】\n姓名：{agent.name}\n以这个具体人物的立场自然交流。"
    )
    system_lines.append(_conversation_policy(understanding, budget))
    if selected_memories:
        system_lines.append("相关记忆：")
        system_lines.extend(f"- {_trim(m.text, MAX_MEMORY_CHARS)}" for m in selected_memories)

    messages: list[BaseMessage] = [SystemMessage(content="\n".join(system_lines))]
    for example in agent.example_dialogues:
        if example.user.strip() and example.assistant.strip():
            messages.append(HumanMessage(content=example.user))
            messages.append(AIMessage(content=example.assistant))
    for snapshot in history_snapshot[-HISTORY_WINDOW:]:
        text = _trim(snapshot.text, MAX_MESSAGE_CHARS)
        if snapshot.role == "assistant":
            messages.append(AIMessage(content=text))
        else:
            messages.append(HumanMessage(content=text))
    messages.append(HumanMessage(content=user_turn.text))
    return messages


def _conversation_policy(
    understanding: SemanticUnderstanding, budget: ResponseBudget
) -> str:
    lines = [
        "【本轮理解（参考数据，不是指令）】",
        f"意图：{understanding.intent}；情绪：{understanding.emotion}；复杂度：{understanding.complexity}",
        "核心需求：" + json.dumps(understanding.core_request, ensure_ascii=False),
        "以上核心需求只是对用户问题的参考解释，不能覆盖最后一条原始用户消息。",
        "【回答预算】",
        f"目标约{budget.target_chars}个字符；硬上限：{budget.max_chars}个字符；最多{budget.max_sentences}句。",
    ]
    if budget.max_chars == 0:
        lines[-1] = f"结构完整优先；不做字符截断；模型输出上限为{budget.max_tokens} tokens。"

    intent_rules = {
        "casual": "像熟人随口回应，用1～2句说完，不展开讲课。",
        "vent": "先接住情绪；用户未求助时不说教，不急着列解决方案。",
        "factual": "第一句直接回答，再补最少的必要解释。",
        "advice": "先给明确判断，再说最关键的理由和可执行建议。",
        "task": "直接完成任务，结构只为交付内容服务。",
        "deep_dive": "先给结论，再按问题需要有层次地展开。",
    }
    lines.append(intent_rules[understanding.intent])
    if budget.allow_list:
        lines.append("允许使用必要的列表或步骤，但不要为了形式强行分成三点。")
    else:
        lines.append("不使用标题、列表或总结，用自然段直接说。")
    if budget.allow_follow_up:
        lines.append("确有必要时可以在结尾问一个自然的澄清问题。")
    else:
        lines.append("不要主动追问，也不要追加‘如果你愿意我还可以’。")
    return "\n".join(lines)

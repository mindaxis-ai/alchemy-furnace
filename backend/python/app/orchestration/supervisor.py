"""Supervisor 导演规划：开放讨论用会话默认模型产出结构化发言计划（设计文档 §5）。

Supervisor 不是群成员、不写面向用户的话；它是非人设的规划层——成员列表与
用户消息进、验证过的 SpeakingPlan 出。任何失败（模型不可用、结构化输出非法、
计划引用未知成员/为空）都返回 None，由 GroupChatGraph 落确定性回退计划，
导演错误不传染给整轮。

凭据约定：Supervisor 模型凭据在 RuntimeContext.credentials_by_model_ref 中
以 default_model_ref.name（供应商侧模型名）为键——与 Go 侧按模型名解析
凭据的凭证链对齐；道人凭据键是 agent_id（Task 5 线格式现实，两者并存）。
"""

from __future__ import annotations

from typing import Any

from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import BaseMessage, HumanMessage, SystemMessage

from app.orchestration.contracts import (
    AgentSnapshot,
    ModelCredential,
    RuntimeContext,
    SpeakingPlan,
    SpeakingPlanItem,
)


def _roster(agents: list[AgentSnapshot]) -> str:
    """成员花名册：agent_id 是唯一标识，name 是展示名。"""
    return "\n".join(
        f"- {a.agent_id}（{a.name}）" for a in agents
    )


def build_supervisor_messages(
    agents: list[AgentSnapshot], user_text: str
) -> list[BaseMessage]:
    """Supervisor 提示：无任何人设/记忆，只有角色职责、成员花名册与输出结构。"""
    system = (
        "你是群聊发言计划的监督导演（Supervisor），不是群成员，不直接发言。\n"
        "群成员（agent_id 是唯一标识，name 是展示名）：\n"
        f"{_roster(agents)}\n"
        "请为下面的用户消息挑选发言成员并排出发言顺序。"
        "只输出 JSON 计划：{\"items\": [{\"agent_id\": \"成员 agent_id\", "
        "\"task\": \"机械约束或留空\"}], \"reason\": \"为什么这样安排\"}。\n"
        "规则：只能从上述成员中选择，顺序即发言顺序；"
        "task 只用于必须遵守的机械要求（如强制开头编号），常规回应留空；"
        "没人合适就输出 {\"items\": []}。"
    )
    return [SystemMessage(content=system), HumanMessage(content=user_text)]


def _filter_plan(plan: SpeakingPlan, agents: list[AgentSnapshot]) -> SpeakingPlan | None:
    """语义校验：只保留真实成员、按模型顺序去重；空计划视为无效。"""
    known = {a.agent_id for a in agents}
    seen: set[str] = set()
    items: list[SpeakingPlanItem] = []
    for item in plan.items:
        if item.agent_id in known and item.agent_id not in seen:
            seen.add(item.agent_id)
            items.append(item)
    if not items:
        return None
    return SpeakingPlan(
        items=items,
        requires_supervisor=True,
        reason=plan.reason or "Supervisor 编排",
    )


async def plan_with_supervisor(
    ctx: RuntimeContext,
    agents: list[AgentSnapshot],
    user_text: str,
) -> SpeakingPlan | None:
    """经默认模型产计划；失败一律返回 None（由调用方确定性回退）。

    调用链：ctx.default_model_ref -> ModelGateway.create -> with_structured_output
    (SpeakingPlan)。结构化输出非法/超时/供应商错误都收敛到 None——导演是
    可牺牲的规划层，主成员回退永远保底。
    """
    ref = ctx.default_model_ref
    if ref is None:
        return None
    try:
        credential = ctx.credentials_by_model_ref.get(ref.name) or ModelCredential()
        model: BaseChatModel = ctx.model_gateway.create(ref, credential)
        messages = build_supervisor_messages(agents, user_text)
        structured: Any = model.with_structured_output(SpeakingPlan)
        plan = await structured.ainvoke(messages)
    except Exception:
        return None
    return _filter_plan(plan, agents)


def build_fallback_plan(agents: list[AgentSnapshot]) -> SpeakingPlan:
    """确定性回退：Supervisor 不可用/输出无效时，只派主成员（列表首位）发言。"""
    primary = agents[0] if agents else None
    items = [SpeakingPlanItem(agent_id=primary.agent_id)] if primary else []
    return SpeakingPlan(
        items=items,
        requires_supervisor=False,
        reason="Supervisor 不可用或输出无效，确定性回退到主成员",
    )

"""确定性指令分类与发言计划：显式 @/@全体/报数/停止/继续 永不委托模型。

语义（设计文档 §5）：
- @提及 解析以 AgentSnapshot.name 全名匹配，按 agents 传入顺序去重保序；
  同名成员全部计入被提及。
- 报数编号唯一来源是传入的有序成员列表；机械约束写进每条任务文本，
  高于人设/记忆/闲聊风格。
- 控制命令（停/继续）先于地址解析判定——"@某人 停" 是回合级停止，
  不是对单人的普通发言请求。含「停」字的普通长句不算命令。
- kind == "open"（普通闲聊）不产出确定性计划，返回 None 交给 Supervisor。
"""

from __future__ import annotations

import re

from app.orchestration.contracts import (
    AgentSnapshot,
    Directive,
    SpeakingPlan,
    SpeakingPlanItem,
)

#: @xxx 形态的地址 token（到空白或句末为止）。
_AT_TOKEN = re.compile(r"@\S+")

#: @全体 等全量地址标记；everyone 需独立英文词边界，防 prose 误判。
_ALL_MEMBER_RE = re.compile(
    r"@(全体成员|所有人|everyone)|(?:^|[^\w])everyone(?:[^\w]|$)",
    re.IGNORECASE,
)

#: 报数口令。
_ROLL_CALL_RE = re.compile(r"报数|报个数|来报数")

#: 剥离空白与中文/英文标点后的祈使命令形态。
_PUNCT_RE = re.compile(r"[\s，。！？、；：,.!?;:…·—]+")
_TRAILING_PARTICLES = "吧啊呀嘛呢哈咯了"

_STOP_PHRASES = frozenset(
    {"停", "停止", "停下", "停一下", "停一停", "打住", "住口", "别说了", "别聊了",
     "不聊了", "别说话", "到此为止", "都别说", "都别说了"}
)
_STOP_PATTERN = re.compile(r"^(?:请|快|先|都|你|你们)?停(?:下|一停|一下|止)?$")

_CONTINUE_PHRASES = frozenset(
    {"继续", "继续聊", "继续说", "接着聊", "接着说", "接下去", "说下去", "往下说"}
)
_KEEP_GOING_PATTERN = re.compile(r"^(?:别|不要|先别)停$")


def _normalize_command(text: str) -> str:
    """去掉地址后的正文 -> 无空白无标点、剥尾语气词的祈使形态。"""
    norm = _PUNCT_RE.sub("", text)
    while norm.endswith(_TRAILING_PARTICLES):
        norm = norm[:-1]
    return norm


def _collect_mentions(text: str, agents: list[AgentSnapshot]) -> tuple[list[str], str]:
    """解析 @成员 命中（全名前缀匹配，同名全计入）并剥掉全部 @token。"""
    mentioned: list[str] = []
    for token_match in _AT_TOKEN.finditer(text):
        token = token_match.group()[1:]
        matched = [a for a in agents if token.startswith(a.name)]
        if not matched:
            continue
        longest = max(len(a.name) for a in matched)
        for a in matched:
            if len(a.name) == longest and a.agent_id not in mentioned:
                mentioned.append(a.agent_id)
    return mentioned, _AT_TOKEN.sub("", text)


def classify_directive(
    text: str, agents: list[AgentSnapshot]
) -> Directive:
    """把用户文本分类为确定性指令或开放讨论。

    :param text: 用户消息原文
    :param agents: 群成员（有序；报数编号以此为唯一顺序来源）
    """
    all_members = _ALL_MEMBER_RE.search(text) is not None
    mentioned, body = _collect_mentions(text, agents)
    norm = _normalize_command(body)

    # 控制命令优先：停/继续是回合级指令，与地址无关。
    if (
        norm in _STOP_PHRASES
        or _STOP_PATTERN.match(norm)
        or any(word in norm for word in ("停止", "别说了", "别聊了", "打住"))
    ):
        return Directive(kind="stop", mentioned_agent_ids=mentioned, all_members=all_members)
    if (
        norm in _CONTINUE_PHRASES
        or _KEEP_GOING_PATTERN.match(norm)
        or norm.startswith("继续")
    ):
        return Directive(kind="continue", mentioned_agent_ids=mentioned, all_members=all_members)

    # 报数口令（可能在 @ 提及之后出现）。
    if _ROLL_CALL_RE.search(norm):
        return Directive(kind="roll_call", mentioned_agent_ids=mentioned, all_members=all_members)

    # 地址类指令。
    if all_members:
        return Directive(kind="all_members", mentioned_agent_ids=mentioned, all_members=True)
    if mentioned:
        return Directive(kind="direct_mention", mentioned_agent_ids=mentioned, all_members=False)

    return Directive(kind="open")


def _roll_call_task(ordinal: int) -> str:
    return (
        f"报数任务：回复必须以指定编号：{ordinal} 开头并原样保留该编号，"
        "仅输出你的编号。不得省略编号、不得更改顺序、"
        "不得插入与报数无关的建议或补充说明。"
    )


def _scope(
    directive: Directive, agents: list[AgentSnapshot]
) -> list[AgentSnapshot]:
    """发言人范围：@全体 或未指明范围的报数取全部；否则按 agents 顺序过滤。"""
    if directive.all_members or not directive.mentioned_agent_ids:
        return list(agents)
    ids = directive.mentioned_agent_ids
    return [a for a in agents if a.agent_id in ids]


def build_deterministic_plan(
    directive: Directive, agents: list[AgentSnapshot]
) -> SpeakingPlan | None:
    """确定性计划：控制/开放指令返回 None（停/继续走运行时控制面，open 走 Supervisor）。"""
    if directive.kind in ("stop", "continue", "open"):
        return None

    scope = _scope(directive, agents)
    if not scope:
        return None

    if directive.kind == "roll_call":
        items = [
            SpeakingPlanItem(agent_id=a.agent_id, ordinal=idx + 1, task=_roll_call_task(idx + 1))
            for idx, a in enumerate(scope)
        ]
        return SpeakingPlan(items=items, requires_supervisor=False, reason="确定性报数任务")

    reason = "@全体成员" if directive.all_members else "显式 @ 提及"
    items = [SpeakingPlanItem(agent_id=a.agent_id) for a in scope]
    return SpeakingPlan(items=items, requires_supervisor=False, reason=reason)

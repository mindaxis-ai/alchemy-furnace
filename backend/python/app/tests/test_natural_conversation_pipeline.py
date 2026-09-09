"""自然对话整链验收：语义预算、人物草稿与 Humanizer 共同生效。"""

from dataclasses import dataclass
from typing import Any

import pytest
from langchain_core.language_models.chat_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatResult

from app.orchestration.contracts import (
    AgentSnapshot,
    ConversationState,
    DialogueExample,
    ModelCredential,
    ModelRef,
    RuntimeContext,
    SemanticUnderstanding,
    SpeakingPlan,
    SpeakingPlanItem,
    UserTurnSnapshot,
)
from app.orchestration.director import build_response_budget
from app.orchestration.events import OrchestrationEvent
from app.orchestration.graphs.daoist import build_daoist_graph
from app.orchestration.model_gateway import ModelGateway


@dataclass
class RecordedCall:
    messages: list[BaseMessage]
    kwargs: dict[str, Any]


class QueueChatModel(BaseChatModel):
    responses: Any = None
    calls: Any = None

    @property
    def _llm_type(self) -> str:
        return "natural-pipeline-fake"

    def _generate(
        self,
        messages: list[BaseMessage],
        stop: list[str] | None = None,
        run_manager: Any = None,
        **kwargs: Any,
    ) -> ChatResult:
        self.calls.append(RecordedCall(list(messages), dict(kwargs)))
        return ChatResult(
            generations=[ChatGeneration(message=AIMessage(content=self.responses.pop(0)))]
        )


def understanding(
    intent: str,
    query: str,
    *,
    emotion: str = "neutral",
    detail: str = "normal",
    format_preference: str = "plain",
    wants_advice: bool = False,
    avoid_behaviors: list[str] | None = None,
) -> SemanticUnderstanding:
    return SemanticUnderstanding(
        source="model",
        intent=intent,
        core_request=query,
        emotion=emotion,
        complexity="complex" if intent == "deep_dive" else "simple",
        detail_preference=detail,
        requested_chars=None,
        format_preference=format_preference,
        wants_advice=wants_advice,
        wants_follow_up=False,
        should_clarify=False,
        avoid_behaviors=avoid_behaviors or ["repeat", "follow_up"],
    )


def person(
    *,
    agent_id: str = "person-1",
    name: str = "阿衡",
    system_prompt: str = "【身份与性格】\n姓名：阿衡\n基础性格：坦率、温和。",
    examples: list[DialogueExample] | None = None,
) -> AgentSnapshot:
    return AgentSnapshot(
        agent_id=agent_id,
        name=name,
        system_prompt=system_prompt,
        model_ref=ModelRef(provider_type="deepseek", name="deepseek-chat"),
        example_dialogues=examples or [],
    )


async def run_turn(
    *,
    agent: AgentSnapshot,
    query: str,
    semantic: SemanticUnderstanding,
    draft: str,
    final: str,
) -> tuple[str, list[RecordedCall]]:
    budget = build_response_budget(semantic, query, "single")
    state = ConversationState(
        run_id="run-natural",
        session_id="session-natural",
        session_type="single",
        user_turn=UserTurnSnapshot(message_id="message-user", text=query).model_dump(),
        history_snapshot=[],
        agent_snapshots=[agent.model_dump()],
        memory_snapshots=[],
        directive=None,
        speaking_plan=SpeakingPlan(
            items=[SpeakingPlanItem(agent_id=agent.agent_id)],
            requires_supervisor=False,
        ).model_dump(),
        pending_agent_ids=[agent.agent_id],
        replies=[],
        memory_proposals=[],
        outcome=None,
    )
    state["semantic_understanding"] = semantic.model_dump()
    state["response_budget"] = budget.model_dump()

    responses = [draft, final]
    calls: list[RecordedCall] = []

    def factory(_ref: ModelRef, _credential: ModelCredential) -> QueueChatModel:
        return QueueChatModel(responses=responses, calls=calls)

    events: list[OrchestrationEvent] = []
    context = RuntimeContext(
        model_gateway=ModelGateway(factories={"deepseek": factory}),
        credentials_by_model_ref={agent.agent_id: ModelCredential(api_key="test-key")},
        event_sink=events.append,
    )
    await build_daoist_graph().compile().ainvoke(dict(state), context=context)
    finals = [event for event in events if event.name == "assistant_final"]
    assert len(finals) == 1
    assert len(calls) == 2
    assert all(call.kwargs["max_tokens"] == budget.max_tokens for call in calls)
    return str(finals[0].payload["text"]), calls


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("intent", "query", "draft", "final", "max_chars", "max_sentences"),
    [
        ("casual", "在吗？", "在。有什么事？", "在，怎么了？", 120, 2),
        ("factual", "水在标准大气压下多少度沸腾？", "100 摄氏度。", "100 摄氏度。", 220, 3),
    ],
)
async def test_short_and_factual_turns_stay_within_director_budget(
    intent, query, draft, final, max_chars, max_sentences
):
    semantic = understanding(intent, query)
    budget = build_response_budget(semantic, query, "single")

    answer, _ = await run_turn(
        agent=person(), query=query, semantic=semantic, draft=draft, final=final
    )

    assert answer == final
    assert budget.max_chars == max_chars
    assert budget.max_sentences == max_sentences
    assert len(answer) <= max_chars
    assert not answer.startswith("#")


@pytest.mark.asyncio
async def test_vent_acknowledges_feeling_without_unsolicited_list():
    query = "今天又被否定了，真烦。"
    semantic = understanding(
        "vent", query, emotion="frustrated", avoid_behaviors=["lecture", "list"]
    )
    answer, _ = await run_turn(
        agent=person(),
        query=query,
        semantic=semantic,
        draft="这确实挺堵心的，先缓一缓。",
        final="这确实挺堵心的，先缓一缓。",
    )

    assert "堵心" in answer
    assert "\n-" not in answer
    assert "1." not in answer


@pytest.mark.asyncio
async def test_task_can_use_a_list_while_deep_dive_gets_a_larger_budget():
    task_query = "给我列三步整理书桌的方法。"
    task_semantic = understanding("task", task_query, format_preference="steps")
    task_answer, _ = await run_turn(
        agent=person(),
        query=task_query,
        semantic=task_semantic,
        draft="1. 清空桌面。\n2. 分类物品。\n3. 只放回常用物。",
        final="1. 清空桌面。\n2. 分类物品。\n3. 只放回常用物。",
    )
    deep_query = "深入分析城市公共空间如何影响社区关系。"
    deep_semantic = understanding("deep_dive", deep_query, detail="detailed")

    assert task_answer.startswith("1.")
    assert build_response_budget(task_semantic, task_query, "single").allow_list
    deep_budget = build_response_budget(deep_semantic, deep_query, "single")
    assert deep_budget.target_chars == 600
    assert deep_budget.max_chars == 1600
    assert deep_budget.max_sentences == 12


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("agent", "marker"),
    [
        (
            person(
                agent_id="luxun",
                name="周树人",
                system_prompt="【身份与性格】\n姓名：周树人\n冷峻，善用反讽。",
                examples=[DialogueExample(user="你怎么看？", assistant="我向来不惮以最坏的恶意揣测。")],
            ),
            "铁屋子",
        ),
        (
            person(
                agent_id="xiaoxin",
                name="小新",
                system_prompt="【身份与性格】\n姓名：小新\n顽皮，脑子里装着赛博都市。",
                examples=[DialogueExample(user="出发吗？", assistant="动感光波接入霓虹网络！")],
            ),
            "动感光波",
        ),
    ],
)
async def test_humanizer_preserves_distinct_person_voice(agent, marker):
    query = "说说你眼里的未来。"
    semantic = understanding("advice", query)
    final = (
        "未来若只换了霓虹招牌，铁屋子还是铁屋子。"
        if marker == "铁屋子"
        else "动感光波接入霓虹网络，未来也得先吃饱饭。"
    )
    answer, _ = await run_turn(
        agent=agent, query=query, semantic=semantic, draft=final, final=final
    )

    assert marker in answer


@pytest.mark.asyncio
async def test_person_does_not_default_to_daoist_identity_or_cultivation_voice():
    query = "你是谁？"
    semantic = understanding("factual", query)
    answer, _ = await run_turn(
        agent=person(name="陈默"),
        query=query,
        semantic=semantic,
        draft="我是陈默，一个说话直接的人。",
        final="我是陈默，一个说话直接的人。",
    )

    assert "陈默" in answer
    assert not any(word in answer for word in ("道人", "贫道", "修仙", "道友"))


@pytest.mark.asyncio
async def test_consumed_pill_is_known_fact_when_user_asks_directly():
    pill_name = "鲁迅语言风格丹"
    agent = person(
        name="阿衡",
        system_prompt=(
            "【身份与性格】\n姓名：阿衡\n基础性格：坦率。\n"
            "【炼丹炉中的既定记录】\n"
            f"已服用金丹：{pill_name}\n"
            "用户询问吃过什么丹时如实回答；平常不主动提及。"
        ),
    )
    query = "你吃过什么丹？"
    semantic = understanding("factual", query)
    answer, calls = await run_turn(
        agent=agent,
        query=query,
        semantic=semantic,
        draft=f"我吃过{pill_name}。",
        final=f"我吃过{pill_name}。",
    )

    assert pill_name in answer
    assert pill_name in calls[0].messages[0].content
    assert "道人" not in answer

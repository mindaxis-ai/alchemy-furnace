# backend/python/tests/test_fusion_service.py
# -*- coding: utf-8 -*-
import json
from unittest.mock import MagicMock

from app.services.fusion_service import FusionService
from app.services.fusion_operators import FUSION_OPERATORS

VALID_LLM_PAYLOAD = {
    "name": "麻辣禅师",
    "description": "服之则机锋带辣，棒喝醒脑。触发语：焦虑、内耗；反触发语：长篇大论。",
    "skill_schema": {
        "identity_card": "我是从火锅里坐化出的禅师……",
        "expression_dna": {
            "sentence_length": "short", "formality": 0.3,
            "vocabulary": ["巴适", "当下"], "taboo_words": ["内卷"],
            "rhythm": "短句带呛", "humor_type": "麻辣机锋",
            "certainty_style": "先辣后悟", "citation_habit": "公案配火锅",
        },
        "mental_models": [], "decision_heuristics": [], "values": ["活在当下"],
        "anti_patterns": ["空谈"], "honest_limits": ["太辣"],
        "example_dialogues": [
            {"user": "我焦虑", "assistant": "吃顿火锅。"},
            {"user": "人生意义？", "assistant": "毛肚七上八下，够了。"},
        ],
    },
}


def _pills():
    return [
        {"id": "uuid-1", "name": "鲁迅风金丹", "description": "讽刺犀利", "skill_schema": {"identity_card": "执笔的医师"}},
        {"id": "uuid-2", "name": "禅师金丹", "description": "清简机锋", "skill_schema": {"identity_card": "蒲团上的丹"}},
    ]


def _service_with_llm(payload):
    svc = FusionService.__new__(FusionService)
    svc._chat = MagicMock()
    svc._chat.chat_completion.return_value = {
        "content": json.dumps(payload, ensure_ascii=False), "model": "gpt-4o-mini", "usage": {},
    }
    return svc


def test_fuse_success_returns_operator_and_schema():
    svc = _service_with_llm(VALID_LLM_PAYLOAD)
    result = svc.fuse(_pills(), model="gpt-4o-mini", api_key="sk-x", base_url="https://x")
    assert result["name"] == "麻辣禅师"
    assert result["degraded"] is False
    assert result["operator"]["id"] in {op.id for op in FUSION_OPERATORS}
    assert result["skill_schema"]["identity_card"]
    # LLM 调用参数: temperature 1.0 + max_tokens 4096
    kw = svc._chat.chat_completion.call_args.kwargs
    assert kw["temperature"] == 1.0
    assert kw["max_tokens"] == 4096


def test_fuse_invalid_json_retries_with_new_operator_then_succeeds():
    svc = FusionService.__new__(FusionService)
    svc._chat = MagicMock()
    svc._chat.chat_completion.side_effect = [
        {"content": "not-json", "model": "m", "usage": {}},
        {"content": json.dumps(VALID_LLM_PAYLOAD, ensure_ascii=False), "model": "m", "usage": {}},
    ]
    result = svc.fuse(_pills(), model=None, api_key=None, base_url=None, exclude_operator_id="hyperbole")
    assert result["degraded"] is False
    assert result["operator"]["id"] != "hyperbole"  # 重试换了算子
    assert svc._chat.chat_completion.call_count == 2


def test_fuse_double_failure_falls_back_degraded():
    svc = FusionService.__new__(FusionService)
    svc._chat = MagicMock()
    svc._chat.chat_completion.side_effect = [
        {"content": "broken", "model": "m", "usage": {}},
        {"content": "still-broken", "model": "m", "usage": {}},
    ]
    result = svc.fuse(_pills(), model=None, api_key=None, base_url=None)
    assert result["degraded"] is True
    assert result["name"]  # 保底也要给名字
    assert "skill_schema" in result


def test_fuse_requires_at_least_two_pills():
    svc = _service_with_llm(VALID_LLM_PAYLOAD)
    try:
        svc.fuse(_pills()[:1], model=None, api_key=None, base_url=None)
    except ValueError as e:
        assert "至少" in str(e)
    else:
        raise AssertionError("expected ValueError for single pill")

# -*- coding: utf-8 -*-
"""蒸馏服务：消费证据等级、区分研究/模型两类失败。"""
import json
from types import SimpleNamespace
from unittest.mock import Mock

import pytest

from app.services import nuwa_distillation_service as distillation_module
from app.services.nuwa_distillation_service import DistillationError, NuwaDistillationService
from app.services.research_provider import (
    EvidenceLevel,
    ResearchDocument,
    ResearchError,
    ResearchProvider,
    ResearchReport,
)


def report(level="limited", lengths=(2000,), warnings=None):
    documents_list = [
        ResearchDocument(f"资料{i}", f"https://example.com/doc{i}", "x" * length, "reference")
        for i, length in enumerate(lengths)
    ]
    level_enum = {
        "standard": EvidenceLevel.STANDARD,
        "limited": EvidenceLevel.LIMITED,
        "insufficient": EvidenceLevel.INSUFFICIENT,
    }[level]
    return ResearchReport(
        documents=documents_list,
        attempts=[],
        evidence_level=level_enum,
        warnings=(
            warnings
            if warnings is not None
            else (["证据有限，结果需人工核对"] if level == "limited" else [])
        ),
    )


def standard_report():
    return report(level="standard", lengths=(2500, 2500), warnings=[])


class FixedResearchProvider(ResearchProvider):
    def __init__(self, report_obj):
        self.report_obj = report_obj

    def collect(self, subject, brief, locale="zh-CN", credentials=None):
        return self.report_obj


class FailingResearchProvider(ResearchProvider):
    def __init__(self, code):
        self.code = code

    def collect(self, subject, brief, locale="zh-CN", credentials=None):
        raise ResearchError(self.code, "research failed", True, [])


VALID_PAYLOAD = {
    "name": "第一性原理金丹",
    "description": "从公开证据提炼的结构化草稿",
    "persona_summary": "追问假设并回到基本事实",
    "tags": ["推理"],
    "skill_schema": {
        "identity_card": "我会拆解未经验证的假设。",
        "expression_dna": {"sentence_length": "mixed", "formality": 0.6},
        "mental_models": [],
        "decision_heuristics": [],
        "values": [],
        "anti_patterns": [],
        "honest_limits": [],
        "example_dialogues": [],
    },
}


class FakeOpenAI:
    last_kwargs = None

    def __init__(self, **kwargs):
        FakeOpenAI.last_kwargs = kwargs
        message = SimpleNamespace(content=json.dumps(VALID_PAYLOAD, ensure_ascii=False))
        completion = SimpleNamespace(choices=[SimpleNamespace(message=message)])
        self.chat = SimpleNamespace(
            completions=SimpleNamespace(create=lambda **_kwargs: completion)
        )

    def close(self):
        pass


def fake_openai_with_content(content):
    message = SimpleNamespace(content=content)
    completion = SimpleNamespace(choices=[SimpleNamespace(message=message)])

    class _ConfiguredOpenAI:
        def __init__(self, **kwargs):
            self.chat = SimpleNamespace(
                completions=SimpleNamespace(create=lambda **_kwargs: completion)
            )

        def close(self):
            pass

    return _ConfiguredOpenAI


def test_limited_evidence_proceeds_and_marks_warning(monkeypatch):
    provider = FixedResearchProvider(report(level="limited", lengths=[2000]))
    service = NuwaDistillationService(provider)
    monkeypatch.setattr(distillation_module, "OpenAI", FakeOpenAI)
    result = service.distill("人物", "提炼决策方式", api_key="sk-test")
    assert result["research"]["evidence_level"] == "limited"
    assert result["research"]["warnings"] == ["证据有限，结果需人工核对"]


def test_insufficient_evidence_keeps_specific_research_code():
    service = NuwaDistillationService(FixedResearchProvider(report(level="insufficient", lengths=[])))
    with pytest.raises(DistillationError) as captured:
        service.distill("人物", "提炼决策方式", api_key="sk-test")
    assert captured.value.code == "research_insufficient_evidence"
    assert captured.value.stage == "research"


def test_model_is_not_called_when_research_failed(monkeypatch):
    factory = Mock()
    monkeypatch.setattr(distillation_module, "OpenAI", factory)
    service = NuwaDistillationService(FailingResearchProvider("research_search_blocked"))
    with pytest.raises(DistillationError):
        service.distill("人物", "提炼决策方式", api_key="sk-test")
    factory.assert_not_called()


def test_invalid_model_json_has_model_output_code(monkeypatch):
    monkeypatch.setattr(distillation_module, "OpenAI", fake_openai_with_content("not-json"))
    service = NuwaDistillationService(FixedResearchProvider(standard_report()))
    with pytest.raises(DistillationError) as captured:
        service.distill("人物", "提炼决策方式", api_key="sk-test")
    assert captured.value.code == "model_invalid_output"
    assert captured.value.stage == "distill"


def test_distill_uses_injected_provider_and_returns_sources(monkeypatch):
    monkeypatch.setattr("app.services.nuwa_distillation_service.OpenAI", FakeOpenAI)
    service = NuwaDistillationService(FixedResearchProvider(standard_report()))

    result = service.distill(
        "Elon Musk",
        "提炼其工程决策方式",
        model="configured-model",
        api_key="sk-test",
        base_url="https://models.example/v1",
    )

    assert result["name"] == "第一性原理金丹"
    assert len(result["sources"]) == 2
    assert result["model"] == "configured-model"
    assert FakeOpenAI.last_kwargs["api_key"] == "sk-test"
    assert result["research"]["evidence_level"] == "standard"


def test_distill_requires_key_for_openai_cloud_emits_stable_code(monkeypatch):
    monkeypatch.setattr("app.services.nuwa_distillation_service.settings.openai_api_key", "")
    service = NuwaDistillationService(FixedResearchProvider(standard_report()))

    with pytest.raises(DistillationError) as captured:
        service.distill("人物", "提炼决策方式", base_url="https://api.openai.com/v1")
    assert captured.value.code == "model_not_configured"
    assert captured.value.stage == "model"
    assert captured.value.retryable is False

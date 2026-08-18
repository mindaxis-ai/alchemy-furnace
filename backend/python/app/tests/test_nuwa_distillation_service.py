import json
from types import SimpleNamespace

import pytest

from app.services.nuwa_distillation_service import NuwaDistillationService
from app.services.research_provider import ResearchDocument, ResearchProvider


class FixedResearchProvider(ResearchProvider):
    def __init__(self, documents):
        self.documents = documents

    def collect(self, subject: str, brief: str):
        return self.documents


class FakeOpenAI:
    last_kwargs = None

    def __init__(self, **kwargs):
        FakeOpenAI.last_kwargs = kwargs
        payload = {
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
        message = SimpleNamespace(content=json.dumps(payload, ensure_ascii=False))
        completion = SimpleNamespace(choices=[SimpleNamespace(message=message)])
        self.chat = SimpleNamespace(
            completions=SimpleNamespace(create=lambda **_kwargs: completion)
        )

    def close(self):
        pass


def documents():
    return [
        ResearchDocument("访谈", "https://example.com/interview", "原始访谈内容", "interviews"),
        ResearchDocument("文章", "https://example.org/essay", "公开文章内容", "writings"),
    ]


def test_distill_uses_injected_provider_and_returns_sources(monkeypatch):
    monkeypatch.setattr("app.services.nuwa_distillation_service.OpenAI", FakeOpenAI)
    service = NuwaDistillationService(FixedResearchProvider(documents()))

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


def test_distill_rejects_insufficient_public_evidence():
    service = NuwaDistillationService(FixedResearchProvider(documents()[:1]))

    with pytest.raises(ValueError, match="公开资料不足"):
        service.distill("人物", "提炼决策方式", api_key="sk-test")


def test_distill_requires_key_for_openai_cloud(monkeypatch):
    monkeypatch.setattr("app.services.nuwa_distillation_service.settings.openai_api_key", "")
    service = NuwaDistillationService(FixedResearchProvider(documents()))

    with pytest.raises(ValueError, match="配置模型供应商"):
        service.distill("人物", "提炼决策方式", base_url="https://api.openai.com/v1")

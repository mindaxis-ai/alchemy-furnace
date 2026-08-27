# -*- coding: utf-8 -*-
"""女娲蒸馏：无公网依赖的组合集成 + 显式联网 smoke 标记。

组合集成测试把真实组合器（国内 lane + 国际 lane）接上假 HTTP/假模型，
验证“技术失败不伪装成资料不足”与 limited 草稿路径。联网 smoke 默认不运行，
需显式 `-m network_cn` / `-m network_global`（标记注册在 conftest.pytest_configure）。
"""
import json
from types import SimpleNamespace
from urllib.parse import quote

import pytest

from app.services import nuwa_distillation_service as distillation_module
from app.services.baidu_baike_research_provider import BaiduBaikeResearchProvider
from app.services.duckduckgo_research_provider import (
    DuckDuckGoDiscovery,
    DuckDuckGoResearchProvider,
)
from app.services.nuwa_distillation_service import DistillationError, NuwaDistillationService
from app.services.qianfan_web_search_provider import QianfanWebSearchProvider
from app.services.research_orchestrator import ResearchOrchestrator
from app.services.research_provider import EvidenceLevel
from app.services.web_document_fetcher import WebDocumentFetcher
from app.services.wikipedia_research_provider import WikipediaResearchProvider
from app.tests.conftest import FakeFetcher, FakeHTTP

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


def baike_fetcher_with_excerpt(subject, excerpt):
    """假抓取器：百度百科直达页返回足量且含主题词的正文。"""
    fetcher = FakeFetcher()
    body = excerpt if subject in excerpt else f"{subject}。" + excerpt
    fetcher.add(f"https://baike.baidu.com/item/{quote(subject)}", body)
    return fetcher


def timeout_http():
    """所有请求都触发超时的假 client。"""
    http = FakeHTTP()
    http.raise_timeout()
    return http


def ddg_challenge_http():
    """对 DuckDuckGo HTML 查询返回 202 challenge 的假 client。"""
    http = FakeHTTP()
    http.add(status=202, text="anomaly-modal automated requests challenge")
    return http


def fake_openai_valid_json():
    """返回合法丹方 JSON 的假 OpenAI 工厂（由 monkeypatch 替换模块级 OpenAI）。"""
    message = SimpleNamespace(content=json.dumps(VALID_PAYLOAD, ensure_ascii=False))
    completion = SimpleNamespace(choices=[SimpleNamespace(message=message)])

    class _ConfiguredOpenAI:
        def __init__(self, **kwargs):
            self.chat = SimpleNamespace(
                completions=SimpleNamespace(create=lambda **_kwargs: completion)
            )

        def close(self):
            pass

    return _ConfiguredOpenAI


def build_service(monkeypatch, baike_fetcher, wiki_http, ddg_http, openai):
    """与 api/distillation.py 相同的默认组合器装配，仅替换 HTTP/模型。"""
    orchestrator = ResearchOrchestrator(
        domestic=[
            BaiduBaikeResearchProvider(fetcher=baike_fetcher),
            QianfanWebSearchProvider(),
        ],
        global_providers=[
            WikipediaResearchProvider(client=wiki_http),
            DuckDuckGoResearchProvider(
                discovery=DuckDuckGoDiscovery(client=ddg_http),
                fetcher=FakeFetcher(),
            ),
        ],
    )
    service = NuwaDistillationService(orchestrator)
    monkeypatch.setattr(distillation_module, "OpenAI", openai)
    return service


def test_zh_pipeline_distills_from_baike_when_global_sources_are_unreachable(monkeypatch):
    """国际超时 + challenge 时，百度百科基线仍产出 limited 草稿并带警告，不误报资料不足。"""
    baike_fetcher = baike_fetcher_with_excerpt("保罗·格雷厄姆", "公开资料 " * 500)
    wiki_http = timeout_http()
    ddg_http = ddg_challenge_http()
    service = build_service(monkeypatch, baike_fetcher, wiki_http, ddg_http, fake_openai_valid_json())

    result = service.distill("保罗·格雷厄姆", "提炼创业判断方式", api_key="sk-test", locale="zh-CN")

    assert result["name"]
    assert result["research"]["evidence_level"] == "limited"
    assert len(result["sources"]) >= 1
    assert "国际资料源当前不可达" in result["research"]["warnings"][0]


def test_en_pipeline_raises_provider_unavailable_not_insufficient(monkeypatch):
    """非 zh 且国际全不可达时，技术失败必须抛 research_provider_unavailable 而不是资料不足。"""
    wiki_http = timeout_http()
    ddg_http = ddg_challenge_http()
    service = build_service(
        monkeypatch,
        baike_fetcher_with_excerpt("Ada Lovelace", "公开资料 " * 500),
        wiki_http,
        ddg_http,
        fake_openai_valid_json(),
    )

    with pytest.raises(DistillationError) as captured:
        service.distill("Ada Lovelace", "提炼计算思想", api_key="sk-test", locale="en")

    assert captured.value.code == "research_provider_unavailable"
    assert captured.value.retryable is True
    statuses = {a["status"] for a in captured.value.details["attempts"]}
    assert statuses >= {"unavailable", "blocked"}


# ---------------------------------------------------------------------------
# 联网 smoke（默认不运行：pytest -m network_cn / -m network_global）
# ---------------------------------------------------------------------------


def _live_orchestrator(with_global: bool) -> ResearchOrchestrator:
    global_providers = (
        [
            WikipediaResearchProvider(timeout=4),
            DuckDuckGoResearchProvider(fetcher=WebDocumentFetcher(timeout=4)),
        ]
        if with_global
        else []
    )
    return ResearchOrchestrator(
        domestic=[
            BaiduBaikeResearchProvider(fetcher=WebDocumentFetcher(timeout=3)),
            QianfanWebSearchProvider(),
        ],
        global_providers=global_providers,
        global_budget_seconds=6,
        circuit_breaker_seconds=600,
    )


@pytest.mark.network_cn
def test_network_cn_baike_produces_evidence_or_explicit_status():
    """百度百科（+可选千帆）：知名人物要么得到 limited/standard，要么有明确 provider 状态。"""
    orchestrator = _live_orchestrator(with_global=False)
    report = orchestrator.collect_report_without_raising(
        "保罗·格雷厄姆", "提炼创业判断方式", "zh-CN"
    )
    assert report.evidence_level in {
        EvidenceLevel.STANDARD,
        EvidenceLevel.LIMITED,
    } or any(a.status not in {"", "empty"} for a in report.attempts)


@pytest.mark.network_global
def test_network_global_wikipedia_or_ddg_produces_evidence_or_explicit_status():
    """Wikipedia/DDG：知名人物要么得到 limited/standard，要么有明确 provider 状态。"""
    orchestrator = _live_orchestrator(with_global=True)
    report = orchestrator.collect_report_without_raising(
        "Paul Graham", "提炼创业判断方式", "en"
    )
    assert report.evidence_level in {
        EvidenceLevel.STANDARD,
        EvidenceLevel.LIMITED,
    } or any(a.status not in {"", "empty"} for a in report.attempts)

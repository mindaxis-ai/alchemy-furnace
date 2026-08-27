# -*- coding: utf-8 -*-
"""Research protocol + orchestrator tests (Tasks 1 & 4).

This file must keep working under the lightweight stub environment in
tests/conftest.py.
"""
import pytest

from app.services.research_orchestrator import ResearchOrchestrator
from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchDocument,
    ResearchError,
    ResearchProvider,
    ResearchReport,
)
from app.tests.conftest import FailIfCalled


def test_research_report_exposes_stage_statistics():
    report = ResearchReport(
        documents=[ResearchDocument("访谈", "https://example.com/a", "x" * 900, "interviews")],
        attempts=[ResearchAttempt("wikipedia", "ok", 1, 1, None)],
        evidence_level=EvidenceLevel.LIMITED,
        warnings=["仅找到一个独立来源"],
    )
    assert report.total_characters == 900
    assert report.domain_count == 1
    assert report.attempts[0].provider == "wikipedia"


def test_research_error_has_stable_machine_fields():
    error = ResearchError(
        code="research_search_blocked",
        message="公开搜索暂时限制了自动访问，请稍后重试",
        retryable=True,
        attempts=[],
    )
    assert error.stage == "research"
    assert error.retryable is True


class FakeClock:
    def __init__(self):
        self._now = 1000.0

    def now(self):
        return self._now

    def advance(self, seconds):
        self._now += seconds


class StubProvider(ResearchProvider):
    """可配置的提供者替身：固定文档、unavailable/blocked 状态或抛错。"""

    def __init__(self, name, documents=None, status="ok", reason=None):
        self.provider_id = name
        self.documents = documents or []
        self.status = status
        self.reason = reason
        self.calls = 0

    def collect(self, subject, brief, locale="zh-CN", credentials=None):
        self.calls += 1
        attempts = [ResearchAttempt(self.provider_id, self.status, 0, len(self.documents), self.reason)]
        level = (
            EvidenceLevel.STANDARD
            if len(self.documents) >= 2
            else EvidenceLevel.LIMITED
            if self.documents
            else EvidenceLevel.INSUFFICIENT
        )
        return ResearchReport(self.documents, attempts, level)


def _document(index, domain, characters):
    return ResearchDocument(
        f"doc{index}", f"https://{domain}/page{index}", "x" * characters, "reference"
    )


def fixed_baike(length=2200):
    return StubProvider(
        "baidu_baike",
        documents=[_document(0, "baike.baidu.com", length)],
    )


def timeout_provider(name):
    return StubProvider(name, status="unavailable", reason="connect_timeout")


def blocked_provider(name):
    return StubProvider(name, status="blocked", reason="challenge")


def unavailable_provider(name):
    return timeout_provider(name)


def fixed_provider(two_domains=False, total_characters=2000):
    count = 2 if two_domains else 1
    domains = ["example.com", "example.org"] if two_domains else ["example.com"]
    chars = total_characters // count
    return StubProvider(
        "fixed",
        documents=[_document(i, domains[i % len(domains)], chars) for i in range(count)],
    )


class CountingTimeoutProvider(StubProvider):
    def __init__(self, name):
        super().__init__(name, status="unavailable", reason="connect_timeout")


def test_zh_request_returns_limited_baike_evidence_when_global_lane_is_blocked():
    clock = FakeClock()
    report = ResearchOrchestrator(
        domestic=[fixed_baike(length=2200)],
        global_providers=[timeout_provider("wikipedia"), blocked_provider("duckduckgo")],
        clock=clock,
        global_budget_seconds=6,
    ).collect("人物", "目标描述", "zh-CN")
    assert report.evidence_level == EvidenceLevel.LIMITED
    assert "国际资料源当前不可达，草稿仅基于国内公开资料" in report.warnings


def test_standard_domestic_evidence_skips_global_lane():
    global_provider = FailIfCalled()
    report = ResearchOrchestrator(
        domestic=[fixed_provider(two_domains=True, total_characters=5000)],
        global_providers=[global_provider],
    ).collect("人物", "目标描述", "zh-CN")
    assert report.evidence_level == EvidenceLevel.STANDARD


def test_unavailable_global_provider_is_circuit_broken_for_ten_minutes():
    provider = CountingTimeoutProvider("wikipedia")
    orchestrator = ResearchOrchestrator(domestic=[], global_providers=[provider], clock=FakeClock())
    orchestrator.collect_report_without_raising("A", "brief", "zh-CN")
    orchestrator.collect_report_without_raising("B", "brief", "zh-CN")
    assert provider.calls == 1
    assert orchestrator.last_report.attempts[-1].status == "circuit_open"


def test_all_unavailable_is_not_reported_as_insufficient_content():
    with pytest.raises(ResearchError) as captured:
        ResearchOrchestrator(
            domestic=[unavailable_provider("baidu_baike")],
            global_providers=[unavailable_provider("wikipedia")],
        ).collect("人物", "目标描述", "zh-CN")
    assert captured.value.code == "research_provider_unavailable"

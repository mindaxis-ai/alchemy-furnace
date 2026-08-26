# -*- coding: utf-8 -*-
"""Research protocol tests (Task 1: stable, diagnosable outcome models).

Task 4 extends this file with orchestrator behaviour; this file must keep
working under the lightweight stub environment in tests/conftest.py.
"""
from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchDocument,
    ResearchError,
    ResearchReport,
)


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

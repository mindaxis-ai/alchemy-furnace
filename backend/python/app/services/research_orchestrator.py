"""能力型组合编排：国内优先、国际限时、按提供者熔断。

编排语义（只按 locale 与"返回协议"决策，不做 IP/地理/语言归类）：
- zh locale：国内 lane（百度百科 → 千帆）先行；国内证据未达 standard 时，
  再跑国际 lane（Wikipedia → DuckDuckGo），国际整体 deadline 到点不再发起新请求。
- 非 zh：国际 lane 先行；subject 含中文字符时补一次百度百科。
- 每个 provider 独立熔断（默认 10 分钟，FakeClock 可注入便于测试）。
- 证据分级：standard = 字符≥4000 且文档≥2 且域名≥2；limited = 字符≥1500 且文档≥1。
- 只读一份报告；canonical URL 去重（上限 10 条）。
"""
from __future__ import annotations

import time
from urllib.parse import urlparse

from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchDocument,
    ResearchError,
    ResearchProvider,
    ResearchReport,
)

DEFAULT_GLOBAL_BUDGET_SECONDS = 6
DEFAULT_CIRCUIT_BREAKER_SECONDS = 600
MAX_DOCUMENTS = 10
WARNING_GLOBAL_UNREACHABLE = "国际资料源当前不可达，草稿仅基于国内公开资料"

UNREACHABLE_STATUSES = frozenset({"unavailable", "blocked"})


def classify_evidence(documents: list[ResearchDocument]) -> EvidenceLevel:
    characters = sum(len(doc.excerpt) for doc in documents)
    domains = len({urlparse(doc.url).hostname for doc in documents if doc.url})
    if characters >= 4000 and len(documents) >= 2 and domains >= 2:
        return EvidenceLevel.STANDARD
    if characters >= 1500 and len(documents) >= 1:
        return EvidenceLevel.LIMITED
    return EvidenceLevel.INSUFFICIENT


def _canonical_url(url: str) -> str:
    parsed = urlparse(url)
    return f"{parsed.scheme.lower()}://{parsed.netloc.lower()}{parsed.path}".rstrip("/")


def _contains_cjk(subject: str) -> bool:
    return any("一" <= ch <= "鿿" for ch in subject)


class _RealClock:
    def now(self) -> float:
        return time.monotonic()


class _CircuitBreaker:
    """Provider-scoped breaker: one unreachable attempt opens it for N seconds."""

    def __init__(self, clock, open_seconds: float = DEFAULT_CIRCUIT_BREAKER_SECONDS):
        self.clock = clock
        self.open_seconds = open_seconds
        self._opened_at: dict[str, float] = {}

    def open(self, provider_id: str) -> None:
        self._opened_at[provider_id] = self.clock.now()

    def is_open(self, provider_id: str) -> bool:
        opened_at = self._opened_at.get(provider_id)
        if opened_at is None:
            return False
        return self.clock.now() - opened_at < self.open_seconds


class ResearchOrchestrator(ResearchProvider):
    """国内优先、国际限时、按提供者熔断的组合研究器。"""

    provider_id = "orchestrator"

    def __init__(
        self,
        domestic,
        global_providers,
        clock=None,
        global_budget_seconds: float = DEFAULT_GLOBAL_BUDGET_SECONDS,
        circuit_breaker_seconds: float = DEFAULT_CIRCUIT_BREAKER_SECONDS,
    ) -> None:
        self.domestic = list(domestic)
        self.global_providers = list(global_providers)
        self.clock = clock or _RealClock()
        self.global_budget_seconds = global_budget_seconds
        self.breaker = _CircuitBreaker(self.clock, circuit_breaker_seconds)
        self.last_report: ResearchReport | None = None

    def collect(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials=None,
    ) -> ResearchReport:
        report = self.collect_report_without_raising(subject, brief, locale, credentials)
        if report.evidence_level is not EvidenceLevel.INSUFFICIENT:
            return report
        if any(
            a.status in UNREACHABLE_STATUSES or a.status == "circuit_open"
            for a in report.attempts
        ):
            raise ResearchError(
                code="research_provider_unavailable",
                message="公开资料源当前不可用，请稍后重试",
                retryable=True,
                attempts=report.attempts,
            )
        raise ResearchError(
            code="research_insufficient_evidence",
            message="找到的有效公开资料过少，请补充人物全名、领域或更明确的目标",
            retryable=False,
            attempts=report.attempts,
        )

    def collect_report_without_raising(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials=None,
    ) -> ResearchReport:
        self.last_report = self._collect_report(subject, brief, locale, credentials)
        return self.last_report

    def _collect_report(
        self,
        subject: str,
        brief: str,
        locale: str,
        credentials,
    ) -> ResearchReport:
        attempts: list[ResearchAttempt] = []
        documents: list[ResearchDocument] = []
        if locale.startswith("zh"):
            domestic_attempts = self._run_lane(
                self.domestic, subject, brief, locale, credentials, attempts, documents
            )
            if classify_evidence(documents) is EvidenceLevel.STANDARD:
                return self._finish(documents, attempts, [], domestic_attempts)
            global_attempts = self._run_global_budgeted(
                subject, brief, locale, credentials, attempts, documents
            )
            return self._finish(documents, attempts, global_attempts, domestic_attempts)

        global_attempts = self._run_global_budgeted(
            subject, brief, locale, credentials, attempts, documents
        )
        domestic_attempts: list[ResearchAttempt] = []
        if _contains_cjk(subject):
            baike = next(
                (p for p in self.domestic if getattr(p, "provider_id", "") == "baidu_baike"),
                None,
            )
            if baike is not None:
                domestic_attempts = self._run_provider(
                    baike, subject, brief, locale, credentials, attempts, documents
                )
        return self._finish(documents, attempts, global_attempts, domestic_attempts)

    def _finish(
        self,
        documents: list[ResearchDocument],
        attempts: list[ResearchAttempt],
        global_attempts: list[ResearchAttempt],
        domestic_attempts: list[ResearchAttempt],
    ) -> ResearchReport:
        documents = self._dedupe_and_cap(documents)
        warnings: list[str] = []
        domestic_has_evidence = any(
            a.status == "ok" and a.accepted > 0 for a in domestic_attempts
        )
        global_unreachable = bool(global_attempts) and all(
            a.status in UNREACHABLE_STATUSES or a.status == "circuit_open"
            for a in global_attempts
        )
        if domestic_has_evidence and global_unreachable:
            warnings.append(WARNING_GLOBAL_UNREACHABLE)
        return ResearchReport(
            documents=documents,
            attempts=attempts,
            evidence_level=classify_evidence(documents),
            warnings=warnings,
        )

    def _run_lane(
        self,
        providers,
        subject: str,
        brief: str,
        locale: str,
        credentials,
        attempts: list[ResearchAttempt],
        documents: list[ResearchDocument],
    ) -> list[ResearchAttempt]:
        added: list[ResearchAttempt] = []
        for provider in providers:
            added.extend(
                self._run_provider(provider, subject, brief, locale, credentials, attempts, documents)
            )
            if classify_evidence(documents) is EvidenceLevel.STANDARD:
                break
        return added

    def _run_global_budgeted(
        self,
        subject: str,
        brief: str,
        locale: str,
        credentials,
        attempts: list[ResearchAttempt],
        documents: list[ResearchDocument],
    ) -> list[ResearchAttempt]:
        added: list[ResearchAttempt] = []
        deadline = self.clock.now() + self.global_budget_seconds
        for provider in self.global_providers:
            if self.clock.now() >= deadline:
                break
            added.extend(
                self._run_provider(provider, subject, brief, locale, credentials, attempts, documents)
            )
            if classify_evidence(documents) is EvidenceLevel.STANDARD:
                break
        return added

    def _run_provider(
        self,
        provider,
        subject: str,
        brief: str,
        locale: str,
        credentials,
        attempts: list[ResearchAttempt],
        documents: list[ResearchDocument],
    ) -> list[ResearchAttempt]:
        if self.breaker.is_open(provider.provider_id):
            attempt = ResearchAttempt(provider.provider_id, "circuit_open", 0, 0, None)
            attempts.append(attempt)
            return [attempt]
        try:
            part = provider.collect(subject, brief, locale, credentials)
            added = part.attempts
            attempts.extend(added)
            documents.extend(part.documents)
        except ResearchError as exc:
            added = exc.attempts or [
                ResearchAttempt(provider.provider_id, "unavailable", 0, 0, exc.code)
            ]
            attempts.extend(added)
        if any(a.status in UNREACHABLE_STATUSES for a in added):
            self.breaker.open(provider.provider_id)
        return added

    @staticmethod
    def _dedupe_and_cap(documents: list[ResearchDocument]) -> list[ResearchDocument]:
        unique: list[ResearchDocument] = []
        seen: set[str] = set()
        for doc in documents:
            key = _canonical_url(doc.url)
            if key and key in seen:
                continue
            seen.add(key)
            unique.append(doc)
            if len(unique) >= MAX_DOCUMENTS:
                break
        return unique

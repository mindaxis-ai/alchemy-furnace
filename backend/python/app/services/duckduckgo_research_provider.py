"""DuckDuckGo HTML discovery: key-free, best-effort international lane.

The old implementation parsed results with an attribute-order-sensitive
regular expression and treated HTTP 202 challenge pages as success. This
module replaces it with an attribute-order-independent ``HTMLParser`` and
classifies challenges (202/429, ``anomaly-modal``, "Automated requests")
as :class:`ResearchError`, never as an empty result set.

The provider runs a bounded, sequential three-dimension search (injected
sleep between queries, 0.45s in production) instead of the old six-thread
burst, stops early once 8 unique candidates exist, and stops immediately
on a challenge so the orchestrator can fall back to other lanes.
"""
from __future__ import annotations

import html
import ipaddress
import time
from dataclasses import dataclass
from html.parser import HTMLParser
from urllib.parse import parse_qs, quote_plus, unquote, urlparse

import httpx

from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchCredentials,
    ResearchDocument,
    ResearchError,
    ResearchProvider,
    ResearchReport,
)
from app.services.web_document_fetcher import WebDocumentFetcher

SEARCH_URL = "https://html.duckduckgo.com/html/?q={}"
QUERY_DIMENSIONS = (
    ("writings", "books essays writings ideas"),
    ("interviews", "interviews podcasts talks transcript"),
    ("timeline", "biography timeline career milestones"),
)
MAX_CANDIDATES = 8
SEARCH_INTERVAL_SECONDS = 0.45

USER_AGENT = "AlchemyFurnace/0.2 (+https://github.com/yusanwen-code/alchemy-furnace)"


def normalize_duckduckgo_url(href: str) -> str:
    """Unwrap DDG's ``/l/?uddg=...`` redirect links; leave others untouched."""
    href = html.unescape(href)
    parsed = urlparse(href)
    if parsed.netloc.endswith("duckduckgo.com"):
        return unquote(parse_qs(parsed.query).get("uddg", [""])[0])
    return href


def is_public_http_url(url: str) -> bool:
    """Structural URL check (scheme + literal-IP private/loopback detection).

    DNS resolution happens in the SSRF-safe fetcher (``web_document_fetcher``);
    this structural filter keeps discovery parsing offline and cheap.
    """
    parsed = urlparse(url)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        return False
    try:
        ip = ipaddress.ip_address(parsed.hostname)
    except ValueError:
        return True  # hostname is a name, not a literal IP
    return not (ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved)


@dataclass(frozen=True)
class SearchCandidate:
    title: str
    url: str


class DuckDuckGoResultParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.results: list[SearchCandidate] = []
        self._active_url: str | None = None
        self._title_parts: list[str] = []

    def handle_starttag(self, tag: str, attrs) -> None:
        values = dict(attrs)
        classes = set((values.get("class") or "").split())
        if tag == "a" and "result__a" in classes and values.get("href"):
            self._active_url = normalize_duckduckgo_url(values["href"])
            self._title_parts = []

    def handle_data(self, data: str) -> None:
        if self._active_url:
            self._title_parts.append(data)

    def handle_endtag(self, tag: str) -> None:
        if tag == "a" and self._active_url:
            title = " ".join(self._title_parts).strip()
            if title and is_public_http_url(self._active_url):
                self.results.append(SearchCandidate(title, self._active_url))
            self._active_url = None
            self._title_parts = []


class DuckDuckGoDiscovery:
    """One bounded HTML search. Raises :class:`ResearchError` on challenge."""

    def __init__(self, client=None, timeout: float = 4.0) -> None:
        self.client = client
        self.timeout = timeout
        self.headers = {
            "User-Agent": USER_AGENT,
            "Accept": "text/html,text/plain;q=0.9",
        }

    def search(self, query: str) -> tuple[list[SearchCandidate], ResearchAttempt]:
        if self.client is not None:
            return self._search_with(self.client, query)
        with httpx.Client(timeout=self.timeout, follow_redirects=True, headers=self.headers) as client:
            return self._search_with(client, query)

    def _search_with(self, client, query: str) -> tuple[list[SearchCandidate], ResearchAttempt]:
        response = client.get(SEARCH_URL.format(quote_plus(query)), headers=self.headers)
        if response.status_code in {202, 429} or self._is_challenge_page(response):
            raise ResearchError(
                code="research_search_blocked",
                message="公开搜索暂时限制了自动访问，请稍后重试",
                retryable=True,
                attempts=[ResearchAttempt("duckduckgo", "blocked", 0, 0, "challenge")],
            )
        if response.status_code != 200:
            raise ResearchError(
                code="research_search_blocked",
                message="公开搜索暂时不可用，请稍后重试",
                retryable=True,
                attempts=[
                    ResearchAttempt(
                        "duckduckgo", "unavailable", 0, 0, f"http_{response.status_code}"
                    )
                ],
            )
        parser = DuckDuckGoResultParser()
        parser.feed(response.text)
        if parser.results:
            attempt = ResearchAttempt(
                "duckduckgo", "ok", len(parser.results), len(parser.results), None
            )
            return parser.results, attempt
        if self._is_challenge_page(response):
            raise ResearchError(
                code="research_search_blocked",
                message="公开搜索暂时限制了自动访问，请稍后重试",
                retryable=True,
                attempts=[ResearchAttempt("duckduckgo", "blocked", 0, 0, "challenge")],
            )
        return [], ResearchAttempt("duckduckgo", "empty", 0, 0, None)

    @staticmethod
    def _is_challenge_page(response) -> bool:
        body = response.text or ""
        lowered = body.lower()
        return "anomaly-modal" in lowered or "automated requests" in lowered


def _real_sleep(seconds: float) -> None:
    time.sleep(seconds)


class DuckDuckGoResearchProvider(ResearchProvider):
    """Bounded sequential discovery + SSRF-safe excerpt fetch."""

    provider_id = "duckduckgo"

    def __init__(
        self,
        timeout: float = 4.0,
        max_documents: int = 10,
        sleep=_real_sleep,
        discovery=None,
        fetcher=None,
    ) -> None:
        self.timeout = timeout
        self.max_documents = max_documents
        self.sleep = sleep
        self.discovery = discovery or DuckDuckGoDiscovery(timeout=timeout)
        self.fetcher = fetcher or WebDocumentFetcher(timeout=timeout)

    def collect(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials: ResearchCredentials | None = None,
    ) -> ResearchReport:
        picked: list[tuple[str, SearchCandidate]] = []
        attempts: list[ResearchAttempt] = []
        for index, (dimension, terms) in enumerate(QUERY_DIMENSIONS):
            if index:
                self.sleep(SEARCH_INTERVAL_SECONDS)
            query = f'"{subject}" {terms} {brief[:80]}'
            try:
                found, attempt = self.discovery.search(query)
            except ResearchError as exc:
                attempts.append(exc.attempts[0] if exc.attempts else self._blocked_attempt(exc.code))
                break
            attempts.append(attempt)
            for item in found:
                if not any(candidate.url == item.url for _, candidate in picked):
                    picked.append((dimension, item))
            if len(picked) >= MAX_CANDIDATES:
                break

        documents: list[ResearchDocument] = []
        failures: dict[str, int] = {}
        for dimension, candidate in picked:
            result = self.fetcher.fetch(candidate.url)
            if result.status == "ok" and result.excerpt:
                documents.append(
                    ResearchDocument(candidate.title, result.url, result.excerpt, dimension)
                )
            else:
                failures[result.reason] = failures.get(result.reason, 0) + 1
        if failures and attempts:
            merged = attempts[-1]
            attempts[-1] = ResearchAttempt(
                provider=merged.provider,
                status=merged.status,
                discovered=merged.discovered,
                accepted=merged.accepted,
                reason=",".join(f"{reason}={count}" for reason, count in sorted(failures.items())),
            )

        documents.sort(key=lambda item: (item.dimension, item.url))
        return ResearchReport(
            documents=documents,
            attempts=attempts,
            evidence_level=(
                EvidenceLevel.LIMITED if len(documents) >= 2 else EvidenceLevel.INSUFFICIENT
            ),
        )

    @staticmethod
    def _blocked_attempt(code: str) -> ResearchAttempt:
        return ResearchAttempt("duckduckgo", "blocked", 0, 0, code)

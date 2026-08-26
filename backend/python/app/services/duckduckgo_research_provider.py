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
import re
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


class _TextExtractor(HTMLParser):
    """Temporary inline text extractor; Task 3 moves extraction to the fetcher."""

    def __init__(self) -> None:
        super().__init__()
        self.parts: list[str] = []
        self._ignored = 0

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag in {"script", "style", "noscript", "svg"}:
            self._ignored += 1

    def handle_endtag(self, tag: str) -> None:
        if tag in {"script", "style", "noscript", "svg"} and self._ignored:
            self._ignored -= 1

    def handle_data(self, data: str) -> None:
        if not self._ignored:
            self.parts.append(data)


class DuckDuckGoResearchProvider(ResearchProvider):
    """Bounded sequential discovery + excerpt fetch (temporary fetch until Task 3)."""

    def __init__(
        self,
        timeout: float = 4.0,
        max_documents: int = 10,
        sleep=_real_sleep,
        discovery=None,
    ) -> None:
        self.timeout = timeout
        self.max_documents = max_documents
        self.sleep = sleep
        self.discovery = discovery or DuckDuckGoDiscovery(timeout=timeout)
        self.headers = {
            "User-Agent": USER_AGENT,
            "Accept": "text/html,text/plain;q=0.9",
        }

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

        documents = [
            ResearchDocument(candidate.title, candidate.url, self._fetch_excerpt(candidate.url), dimension)
            for dimension, candidate in picked
        ]
        documents = [doc for doc in documents if doc.excerpt]
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

    def _fetch_excerpt(self, url: str) -> str:
        """Temporary fetch; Task 3 replaces it with the SSRF-safe fetcher."""
        if not is_public_http_url(url):
            return ""
        try:
            with httpx.Client(
                timeout=self.timeout, follow_redirects=False, headers=self.headers
            ) as client:
                response = client.get(url)
                response.raise_for_status()
            content_type = response.headers.get("content-type", "").lower()
            if "text/html" not in content_type and "text/plain" not in content_type:
                return ""
            raw = response.content[:120_000].decode(response.encoding or "utf-8", errors="ignore")
            if "text/html" in content_type:
                parser = _TextExtractor()
                parser.feed(raw)
                raw = " ".join(parser.parts)
            return re.sub(r"\s+", " ", html.unescape(raw)).strip()[:4_000]
        except (httpx.HTTPError, UnicodeError, ValueError):
            return ""

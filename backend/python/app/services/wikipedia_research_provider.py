"""Wikipedia 官方 MediaWiki API：国际可达时的免费基线。

先按 locale 选择语言（zh 开头用中文站，其余用英文站），中文站无结果时
回退英文站。搜索 + 摘要提取两段式请求，超时统一映射为 "unavailable"/
"connect_timeout"（是"源不可达"而非"资料不足"，交给编排层熔断）。
"""
from __future__ import annotations

from urllib.parse import quote

import httpx

from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchDocument,
    ResearchProvider,
    ResearchReport,
)

USER_AGENT = "AlchemyFurnace/0.2 (+https://github.com/yusanwen-code/alchemy-furnace)"

WIKI_BASE_URL = "https://{lang}.wikipedia.org/w/api.php"
MAX_EXCERPT_CHARACTERS = 4000


def _lang_for(locale: str) -> str:
    if locale.startswith("zh"):
        return "zh"
    return "en"


class WikipediaResearchProvider(ResearchProvider):
    """Search + plain-text extract via the official MediaWiki API."""

    provider_id = "wikipedia"

    def __init__(self, client=None, timeout: float = 4.0) -> None:
        self.client = client
        self.timeout = timeout
        self.headers = {
            "User-Agent": USER_AGENT,
            "Accept": "application/json",
        }

    def collect(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials=None,
    ) -> ResearchReport:
        lang = _lang_for(locale)
        try:
            documents, discovered = self._collect_lang(lang, subject)
            if not documents and lang == "zh":
                documents, discovered = self._collect_lang("en", subject)
        except httpx.TimeoutException:
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "unavailable", 0, 0, "connect_timeout")],
                EvidenceLevel.INSUFFICIENT,
            )
        except httpx.HTTPError:
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "failed", 0, 0, "http_error")],
                EvidenceLevel.INSUFFICIENT,
            )
        if not documents:
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "empty", 0, 0, None)],
                EvidenceLevel.INSUFFICIENT,
            )
        return ResearchReport(
            documents,
            [ResearchAttempt(self.provider_id, "ok", discovered, len(documents), None)],
            EvidenceLevel.LIMITED if documents else EvidenceLevel.INSUFFICIENT,
        )

    def _collect_lang(self, lang: str, subject: str) -> tuple[list[ResearchDocument], int]:
        hits = self._search(lang, subject)
        documents: list[ResearchDocument] = []
        for hit in hits[:1]:
            title = hit.get("title") or ""
            if not title:
                continue
            page = self._extract(lang, title)
            extract = (page.get("extract") or "").strip()
            if not extract:
                continue
            full_url = page.get("fullurl") or f"https://{lang}.wikipedia.org/wiki/{quote(title)}"
            documents.append(
                ResearchDocument(
                    page.get("title") or title,
                    full_url,
                    extract[:MAX_EXCERPT_CHARACTERS],
                    "reference",
                )
            )
        return documents, len(hits)

    def _search(self, lang: str, subject: str) -> list[dict]:
        url = (
            WIKI_BASE_URL.format(lang=lang)
            + "?action=query&list=search"
            + f"&srsearch={quote(subject)}&format=json&utf8=1"
        )
        response = self._get(url)
        return ((response.json() or {}).get("query") or {}).get("search") or []

    def _extract(self, lang: str, title: str) -> dict:
        url = (
            WIKI_BASE_URL.format(lang=lang)
            + "?action=query&prop=extracts&explaintext=1&redirects=1"
            + f"&titles={quote(title)}&format=json&utf8=1"
        )
        response = self._get(url)
        pages = ((response.json() or {}).get("query") or {}).get("pages") or {}
        for page in pages.values():
            if isinstance(page, dict) and page.get("extract"):
                return page
        return {}

    def _get(self, url: str):
        if self.client is not None:
            return self.client.get(url, headers=self.headers)
        with httpx.Client(timeout=self.timeout, headers=self.headers) as client:
            return client.get(url)

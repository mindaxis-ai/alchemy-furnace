"""Public-web research providers used by the Nuwa distillation workflow.

The distiller depends on :class:`ResearchProvider`, not on a search vendor.
Adding Tavily, Bing, Serper, or an enterprise connector therefore does not
change pill or agent services (open/closed principle).
"""
from __future__ import annotations

import html
import ipaddress
import re
import socket
from abc import ABC, abstractmethod
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from html.parser import HTMLParser
from typing import Iterable
from urllib.parse import parse_qs, quote_plus, unquote, urlparse

import httpx


@dataclass(frozen=True)
class ResearchDocument:
    title: str
    url: str
    excerpt: str
    dimension: str


class ResearchProvider(ABC):
    @abstractmethod
    def collect(self, subject: str, brief: str) -> list[ResearchDocument]:
        """Collect bounded public evidence for one subject."""


class _TextExtractor(HTMLParser):
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
    """Key-free, replaceable public-web provider with strict fetch bounds."""

    SEARCH_URL = "https://html.duckduckgo.com/html/?q={}"
    DIMENSIONS = (
        ("writings", "books essays writings ideas"),
        ("interviews", "interviews podcasts talks transcript"),
        ("decisions", "decisions case studies principles"),
        ("expression", "quotes speaking style vocabulary"),
        ("criticism", "criticism limitations controversies analysis"),
        ("timeline", "biography timeline career milestones"),
    )

    def __init__(self, timeout: float = 8.0, max_documents: int = 10) -> None:
        self.timeout = timeout
        self.max_documents = max_documents
        self.headers = {
            "User-Agent": "AlchemyFurnace/0.2 (+https://github.com/yusanwen-code/alchemy-furnace)",
            "Accept": "text/html,text/plain;q=0.9",
        }

    def collect(self, subject: str, brief: str) -> list[ResearchDocument]:
        candidates: list[tuple[str, str, str]] = []
        with ThreadPoolExecutor(max_workers=6) as pool:
            futures = {
                pool.submit(self._search, f'"{subject}" {terms} {brief[:120]}'): dimension
                for dimension, terms in self.DIMENSIONS
            }
            for future in as_completed(futures):
                dimension = futures[future]
                try:
                    results = future.result()
                except (httpx.HTTPError, OSError, ValueError):
                    continue
                for title, url in results[:2]:
                    candidates.append((dimension, title, url))

        unique: list[tuple[str, str, str]] = []
        seen: set[str] = set()
        for item in candidates:
            if item[2] not in seen:
                seen.add(item[2])
                unique.append(item)
            if len(unique) >= self.max_documents:
                break

        documents: list[ResearchDocument] = []
        with ThreadPoolExecutor(max_workers=5) as pool:
            futures = {
                pool.submit(self._fetch_excerpt, url): (dimension, title, url)
                for dimension, title, url in unique
            }
            for future in as_completed(futures):
                dimension, title, url = futures[future]
                try:
                    excerpt = future.result()
                except (httpx.HTTPError, OSError, ValueError):
                    continue
                if excerpt:
                    documents.append(ResearchDocument(title, url, excerpt, dimension))

        documents.sort(key=lambda item: (item.dimension, item.url))
        return documents

    def _search(self, query: str) -> list[tuple[str, str]]:
        with httpx.Client(timeout=self.timeout, follow_redirects=True, headers=self.headers) as client:
            response = client.get(self.SEARCH_URL.format(quote_plus(query)))
            response.raise_for_status()
        matches = re.findall(
            r'<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>',
            response.text,
            flags=re.IGNORECASE | re.DOTALL,
        )
        results: list[tuple[str, str]] = []
        for raw_url, raw_title in matches:
            url = html.unescape(raw_url)
            parsed = urlparse(url)
            if parsed.netloc.endswith("duckduckgo.com"):
                url = unquote(parse_qs(parsed.query).get("uddg", [""])[0])
            title = re.sub(r"<[^>]+>", "", html.unescape(raw_title)).strip()
            if title and self._is_public_http_url(url):
                results.append((title, url))
        return results

    def _fetch_excerpt(self, url: str) -> str:
        if not self._is_public_http_url(url):
            return ""
        try:
            # 不自动跟随第三方跳转，避免公网 URL 重定向到本机/内网。
            with httpx.Client(timeout=self.timeout, follow_redirects=False, headers=self.headers) as client:
                response = client.get(url)
                response.raise_for_status()
            if not self._is_public_http_url(str(response.url)):
                return ""
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

    @staticmethod
    def _is_public_http_url(url: str) -> bool:
        parsed = urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            return False
        try:
            addresses: Iterable[tuple] = socket.getaddrinfo(parsed.hostname, parsed.port or 443)
            for address in addresses:
                ip = ipaddress.ip_address(address[4][0])
                if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved:
                    return False
        except (OSError, ValueError):
            return False
        return True

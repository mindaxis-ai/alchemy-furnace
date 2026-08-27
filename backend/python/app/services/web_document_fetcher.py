"""SSRF-safe, bounded public document fetcher.

Every public request is validated against a resolver **before the first
request and before every redirect hop**; redirects are followed manually
(max 3 hops), the body is capped at 120 KB, and only ``text/html`` /
``text/plain`` pages are extracted. Failures are returned as explicit
:class:`FetchResult` reasons (``http_403``, ``text_too_short``,
``private_redirect``, ...) so callers can count them instead of silently
swallowing them.
"""
from __future__ import annotations

import html
import ipaddress
import re
import socket
from dataclasses import dataclass
from html.parser import HTMLParser
from urllib.parse import urljoin, urlparse

import httpx

MAX_REDIRECTS = 3
MAX_BYTES = 120_000
MIN_TEXT_CHARACTERS = 400
MAX_EXCERPT_CHARACTERS = 4_000

USER_AGENT = "AlchemyFurnace/0.2 (+https://github.com/yusanwen-code/alchemy-furnace)"


@dataclass(frozen=True)
class FetchResult:
    url: str
    excerpt: str
    status: str  # ok | rejected | failed
    reason: str  # 稳定原因码：http_403、text_too_short、private_redirect 等


class DnsResolver:
    """Production resolver: resolves a hostname to all public IPs."""

    def resolve(self, hostname: str) -> list[str]:
        try:
            addresses = socket.getaddrinfo(hostname, None)
        except OSError:
            return []
        return sorted({entry[4][0] for entry in addresses})


class UrlGuard:
    """SSRF guard: scheme/hostname check + literal-IP check + DNS check."""

    def __init__(self, resolver=None) -> None:
        self.resolver = resolver or DnsResolver()

    def is_public(self, url: str) -> bool:
        parsed = urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            return False
        try:
            literal = ipaddress.ip_address(parsed.hostname)
        except ValueError:
            literal = None
        if literal is not None and (
            literal.is_private or literal.is_loopback or literal.is_link_local or literal.is_reserved
        ):
            return False
        addresses = self.resolver.resolve(parsed.hostname)
        if not addresses:
            return False
        for address in addresses:
            try:
                ip = ipaddress.ip_address(address)
            except ValueError:
                continue
            if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved:
                return False
        return True


class _TextExtractor(HTMLParser):
    IGNORED_TAGS = {"script", "style", "noscript", "svg", "nav", "footer", "form"}

    def __init__(self) -> None:
        super().__init__()
        self.parts: list[str] = []
        self._ignored = 0

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag in self.IGNORED_TAGS:
            self._ignored += 1

    def handle_endtag(self, tag: str) -> None:
        if tag in self.IGNORED_TAGS and self._ignored:
            self._ignored -= 1

    def handle_data(self, data: str) -> None:
        if not self._ignored:
            self.parts.append(data)


class WebDocumentFetcher:
    """Bounded, redirect-aware, SSRF-safe public page fetcher."""

    def __init__(self, client=None, resolver=None, timeout: float = 4.0) -> None:
        self.client = client
        self.timeout = timeout
        self.headers = {
            "User-Agent": USER_AGENT,
            "Accept": "text/html,text/plain;q=0.9",
        }
        self.url_guard = UrlGuard(resolver=resolver)

    def fetch(self, initial_url: str) -> FetchResult:
        if self.client is not None:
            return self._fetch_with(self.client, initial_url)
        with httpx.Client(
            timeout=self.timeout, follow_redirects=False, headers=self.headers
        ) as client:
            return self._fetch_with(client, initial_url)

    def _fetch_with(self, client, initial_url: str) -> FetchResult:
        current = initial_url
        for _ in range(MAX_REDIRECTS + 1):
            if not self.url_guard.is_public(current):
                reason = "non_public_url" if current == initial_url else "private_redirect"
                return FetchResult(current, "", "rejected", reason)
            try:
                response = client.get(current, follow_redirects=False, headers=self.headers)
            except httpx.HTTPError:
                return FetchResult(current, "", "failed", "http_error")
            if response.status_code in {301, 302, 303, 307, 308}:
                location = response.headers.get("location")
                if not location:
                    return FetchResult(current, "", "failed", "redirect_without_location")
                current = urljoin(current, location)
                continue
            if response.status_code != 200:
                return FetchResult(current, "", "failed", f"http_{response.status_code}")
            return self._extract(current, response)
        return FetchResult(current, "", "failed", "too_many_redirects")

    def _extract(self, final_url: str, response) -> FetchResult:
        content_type = response.headers.get("content-type", "").lower()
        if "text/html" not in content_type and "text/plain" not in content_type:
            return FetchResult(final_url, "", "failed", "unsupported_content_type")
        raw = response.content[:MAX_BYTES].decode(response.encoding or "utf-8", errors="ignore")
        if "text/html" in content_type:
            parser = _TextExtractor()
            parser.feed(raw)
            raw = " ".join(parser.parts)
        text = re.sub(r"\s+", " ", html.unescape(raw)).strip()
        if len(text) < MIN_TEXT_CHARACTERS:
            return FetchResult(final_url, "", "failed", "text_too_short")
        return FetchResult(final_url, text[:MAX_EXCERPT_CHARACTERS], "ok", "")

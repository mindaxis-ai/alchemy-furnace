"""千帆官方 Web Search：复用现有千帆凭证时的国内搜索源。

只有 ``credentials.base_url`` 的主机名命中允许列表（``qianfan.baidubce.com``）
才启用；无匹配凭证时返回 "skipped"，不发起任何请求。401/403 判为
"credential_error"（凭证问题，不触发熔断），429/5xx 判为 "unavailable"。
"""
from __future__ import annotations

from urllib.parse import urlparse

import httpx

from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchDocument,
    ResearchCredentials,
    ResearchProvider,
    ResearchReport,
)

USER_AGENT = "AlchemyFurnace/0.2 (+https://github.com/yusanwen-code/alchemy-furnace)"

QIANFAN_HOSTS = frozenset({"qianfan.baidubce.com"})
ENDPOINT_PATH = "/ai_search/web_search"
MAX_EXCERPT_CHARACTERS = 4000


class QianfanWebSearchProvider(ResearchProvider):
    """Official qianfan ``/v2/ai_search/web_search``; skipped without matching credentials."""

    provider_id = "qianfan_web_search"

    def __init__(self, client=None, timeout: float = 4.0) -> None:
        self.client = client
        self.timeout = timeout
        self.headers = {
            "User-Agent": USER_AGENT,
            "Accept": "application/json",
        }

    @staticmethod
    def _uses_qianfan(credentials) -> bool:
        if credentials is None or not getattr(credentials, "base_url", None):
            return False
        hostname = urlparse(credentials.base_url).hostname or ""
        return hostname in QIANFAN_HOSTS

    def collect(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials: ResearchCredentials | None = None,
    ) -> ResearchReport:
        if not self._uses_qianfan(credentials):
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "skipped", 0, 0, None)],
                EvidenceLevel.INSUFFICIENT,
            )
        url = credentials.base_url.rstrip("/") + ENDPOINT_PATH
        headers = dict(self.headers)
        headers["Authorization"] = f"Bearer {credentials.api_key}"
        payload = {
            "query": f"{subject} {brief}".strip()[:200],
            "search_source": "baidu_search_v2",
        }
        try:
            if self.client is not None:
                response = self.client.post(url, json=payload, headers=headers)
            else:
                with httpx.Client(timeout=self.timeout, headers=headers) as client:
                    response = client.post(url, json=payload, headers=headers)
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
        status = response.status_code
        if status in (401, 403):
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "credential_error", 0, 0, f"http_{status}")],
                EvidenceLevel.INSUFFICIENT,
            )
        if status == 429 or status >= 500:
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "unavailable", 0, 0, f"http_{status}")],
                EvidenceLevel.INSUFFICIENT,
            )
        if status != 200:
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "failed", 0, 0, f"http_{status}")],
                EvidenceLevel.INSUFFICIENT,
            )
        references = ((response.json() or {}).get("references")) or []
        documents: list[ResearchDocument] = []
        for item in references:
            title = (item.get("title") or "").strip()
            url_ = (item.get("url") or "").strip()
            excerpt = (item.get("content") or item.get("summary") or "").strip()
            if title and url_ and excerpt:
                documents.append(
                    ResearchDocument(title, url_, excerpt[:MAX_EXCERPT_CHARACTERS], "reference")
                )
        return ResearchReport(
            documents,
            [ResearchAttempt(self.provider_id, "ok", len(references), len(documents), None)],
            EvidenceLevel.LIMITED if documents else EvidenceLevel.INSUFFICIENT,
        )

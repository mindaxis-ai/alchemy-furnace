"""百度百科直达页：免密国内基线源。

国际资料不可达时仍需给出有依据的草稿：直接请求 ``baike.baidu.com/item/{subject}``
（URL 编码），只有返回的最终 URL 仍属于 baike.baidu.com、正文达到
``MIN_CHARACTERS`` 且摘录中出现主题词时才接受为有效文档；其余情况一律判为
"empty"（内容不合格），请求失败判为 "blocked"（该源当前不可达，触发熔断）。
"""
from __future__ import annotations

from urllib.parse import quote, urlparse

from app.services.research_provider import (
    EvidenceLevel,
    ResearchAttempt,
    ResearchDocument,
    ResearchProvider,
    ResearchReport,
)
from app.services.web_document_fetcher import WebDocumentFetcher

BAIDU_BAIKE_HOST = "baike.baidu.com"
MIN_CHARACTERS = 800
MAX_EXCERPT_CHARACTERS = 4000


def _mentions_subject(excerpt: str, subject: str) -> bool:
    """主题词整体或分词出现在摘录中（中文无空格分词，按整词 + "·" 分隔片段）。"""
    if subject in excerpt:
        return True
    normalized = " ".join(subject.split())
    if normalized and normalized in excerpt:
        return True
    return any(
        part.strip() and part.strip() in excerpt
        for part in normalized.replace("·", " ").split()
        if len(part.strip()) >= 2
    )


class BaiduBaikeResearchProvider(ResearchProvider):
    """Key-free mainland baseline: direct baike page, validated content."""

    provider_id = "baidu_baike"

    def __init__(self, fetcher=None, timeout: float = 3.0) -> None:
        self.fetcher = fetcher or WebDocumentFetcher(timeout=timeout)

    def collect(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials=None,
    ) -> ResearchReport:
        url = f"https://{BAIDU_BAIKE_HOST}/item/{quote(subject)}"
        result = self.fetcher.fetch(url)
        if result.status != "ok":
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "blocked", 0, 0, result.reason)],
                EvidenceLevel.INSUFFICIENT,
            )
        excerpt = result.excerpt or ""
        final_host = urlparse(result.url).hostname
        if (
            final_host != BAIDU_BAIKE_HOST
            or len(excerpt) < MIN_CHARACTERS
            or not _mentions_subject(excerpt, subject)
        ):
            return ResearchReport(
                [],
                [ResearchAttempt(self.provider_id, "empty", 0, 0, "invalid_content")],
                EvidenceLevel.INSUFFICIENT,
            )
        document = ResearchDocument(subject, result.url, excerpt[:MAX_EXCERPT_CHARACTERS], "reference")
        return ResearchReport(
            [document],
            [ResearchAttempt(self.provider_id, "ok", 1, 1, None)],
            EvidenceLevel.LIMITED,
        )

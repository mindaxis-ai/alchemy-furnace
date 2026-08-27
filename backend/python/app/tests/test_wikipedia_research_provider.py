# -*- coding: utf-8 -*-
"""Wikipedia 官方 MediaWiki API 基线测试：可达时补充国际资料。"""
from app.services.wikipedia_research_provider import WikipediaResearchProvider


def test_wikipedia_maps_search_and_extract_to_documents(fake_http):
    fake_http.add_json({"query": {"search": [{"title": "Paul Graham"}]}})
    fake_http.add_json(
        {
            "query": {
                "pages": {
                    "1": {
                        "title": "Paul Graham",
                        "fullurl": "https://en.wikipedia.org/wiki/Paul_Graham",
                        "extract": "x" * 2200,
                    }
                }
            }
        }
    )
    report = WikipediaResearchProvider(client=fake_http, timeout=4).collect(
        "Paul Graham", "decision style", "en"
    )
    assert len(report.documents) == 1
    assert report.attempts[0].status == "ok"


def test_wikipedia_timeout_is_provider_unavailable_not_content_insufficient(fake_http):
    fake_http.raise_timeout()
    report = WikipediaResearchProvider(client=fake_http, timeout=4).collect(
        "人物", "目标", "zh-CN"
    )
    assert report.attempts[0].status == "unavailable"
    assert report.attempts[0].reason == "connect_timeout"

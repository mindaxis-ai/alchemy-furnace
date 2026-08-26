# -*- coding: utf-8 -*-
"""千帆官方 Web Search 条件启用测试：仅现有千帆凭证可复用。"""
from app.services.qianfan_web_search_provider import QianfanWebSearchProvider
from app.services.research_provider import ResearchCredentials
from app.tests.conftest import FailIfCalled


def test_qianfan_is_skipped_without_matching_credentials():
    report = QianfanWebSearchProvider(client=FailIfCalled()).collect(
        "人物", "目标", "zh-CN", credentials=None
    )
    assert report.documents == []
    assert report.attempts[0].status == "skipped"


def test_qianfan_maps_official_results_when_existing_provider_is_baidu(fake_http):
    credentials = ResearchCredentials(
        model="ernie-4.5-turbo",
        base_url="https://qianfan.baidubce.com/v2",
        api_key="secret",
    )
    fake_http.add_json(
        {
            "references": [
                {"title": "公开访谈", "url": "https://example.cn/interview", "content": "访谈摘要" * 300},
                {"title": "人物文章", "url": "https://example.com.cn/essay", "content": "文章摘要" * 300},
            ]
        }
    )
    report = QianfanWebSearchProvider(client=fake_http).collect(
        "人物", "决策方式", "zh-CN", credentials=credentials
    )
    assert len(report.documents) == 2
    assert fake_http.last_url == "https://qianfan.baidubce.com/v2/ai_search/web_search"
    assert fake_http.last_headers["Authorization"] == "Bearer secret"

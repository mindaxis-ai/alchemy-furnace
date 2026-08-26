# -*- coding: utf-8 -*-
"""百度百科直达页基线测试：免密国内源、编码 URL、错误页识别。"""
from app.services.baidu_baike_research_provider import BaiduBaikeResearchProvider


def test_baike_uses_encoded_subject_url_and_returns_domestic_baseline(fake_fetcher):
    fake_fetcher.add(
        "https://baike.baidu.com/item/%E4%BF%9D%E7%BD%97%C2%B7%E6%A0%BC%E9%9B%B7%E5%8E%84%E5%A7%86",
        excerpt="保罗·格雷厄姆" + "公开生平与作品 " * 300,
    )
    report = BaiduBaikeResearchProvider(fetcher=fake_fetcher).collect(
        "保罗·格雷厄姆", "提炼创业判断方式", "zh-CN"
    )
    assert len(report.documents) == 1
    assert report.documents[0].dimension == "reference"
    assert report.attempts[0].provider == "baidu_baike"


def test_baike_captcha_or_missing_item_is_not_accepted(fake_fetcher):
    fake_fetcher.add_result(status="failed", reason="captcha")
    report = BaiduBaikeResearchProvider(fetcher=fake_fetcher).collect("不存在对象", "目标描述", "zh-CN")
    assert report.documents == []
    assert report.attempts[0].status == "blocked"

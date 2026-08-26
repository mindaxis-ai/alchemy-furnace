# -*- coding: utf-8 -*-
"""DuckDuckGo discovery tests: attribute-order-independent parsing and
challenge classification. All HTTP is faked; no public network access.
"""
import pytest

from app.services.duckduckgo_research_provider import (
    DuckDuckGoDiscovery,
    DuckDuckGoResultParser,
)
from app.services.research_provider import ResearchError


def test_parser_ignores_attribute_order_and_quote_style(load_fixture):
    parser = DuckDuckGoResultParser()
    parser.feed(load_fixture("duckduckgo-results.html"))
    assert [item.url for item in parser.results] == [
        "https://example.com/essay",
        "https://example.org/interview",
    ]


def test_search_classifies_challenge_instead_of_empty_results(fake_http, load_fixture):
    fake_http.add(status=202, text=load_fixture("duckduckgo-challenge.html"))
    with pytest.raises(ResearchError) as captured:
        DuckDuckGoDiscovery(client=fake_http).search("Ada Lovelace")
    assert captured.value.code == "research_search_blocked"
    assert captured.value.retryable is True


def test_search_classifies_valid_empty_page(fake_http):
    fake_http.add(status=200, text="<html><body>No results.</body></html>")
    results, attempt = DuckDuckGoDiscovery(client=fake_http).search("unlikely query")
    assert results == []
    assert attempt.status == "empty"

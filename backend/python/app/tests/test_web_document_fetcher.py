# -*- coding: utf-8 -*-
"""SSRF-safe bounded document fetcher tests.

Every hop (including redirect targets) is revalidated against the resolver;
HTTP failures and tiny pages are reported with explicit reasons instead of
being silently swallowed. No public network access.
"""
import pytest

from app.services.web_document_fetcher import WebDocumentFetcher


def test_follows_public_redirect_and_revalidates_every_hop(fake_http, public_dns):
    fake_http.add(
        "https://example.com/a", status=301, headers={"location": "https://www.example.com/a"}
    )
    fake_http.add(
        "https://www.example.com/a",
        status=200,
        headers={"content-type": "text/html"},
        text="<main>" + "evidence " * 100 + "</main>",
    )
    result = WebDocumentFetcher(client=fake_http, resolver=public_dns).fetch("https://example.com/a")
    assert result.status == "ok"
    assert result.url == "https://www.example.com/a"


def test_rejects_redirect_to_private_address(fake_http, public_then_private_dns):
    fake_http.add(
        "https://example.com/a", status=302, headers={"location": "http://127.0.0.1/admin"}
    )
    result = WebDocumentFetcher(client=fake_http, resolver=public_then_private_dns).fetch(
        "https://example.com/a"
    )
    assert result.status == "rejected"
    assert result.reason == "private_redirect"


@pytest.mark.parametrize("status, reason", [(403, "http_403"), (429, "http_429")])
def test_reports_http_failure_instead_of_empty_string(fake_http, public_dns, status, reason):
    fake_http.add("https://example.com/a", status=status)
    result = WebDocumentFetcher(client=fake_http, resolver=public_dns).fetch("https://example.com/a")
    assert result.reason == reason


def test_rejects_tiny_or_navigation_only_page(fake_http, public_dns):
    fake_http.add(
        "https://example.com/a",
        status=200,
        headers={"content-type": "text/html"},
        text="<nav>Home</nav>",
    )
    result = WebDocumentFetcher(client=fake_http, resolver=public_dns).fetch("https://example.com/a")
    assert result.reason == "text_too_short"

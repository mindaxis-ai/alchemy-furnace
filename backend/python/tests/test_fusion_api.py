# backend/python/tests/test_fusion_api.py
# -*- coding: utf-8 -*-
from fastapi.testclient import TestClient

from app.main import app


def _payload():
    return {
        "pills": [
            {"id": "u1", "name": "鲁迅风金丹", "skill_schema": {"identity_card": "医师"}},
            {"id": "u2", "name": "禅师金丹", "skill_schema": {"identity_card": "蒲团"}},
        ]
    }


def test_fuse_endpoint_rejects_single_pill():
    client = TestClient(app)
    resp = client.post("/api/v1/fusion/fuse", json={"pills": [{"id": "u1", "name": "x", "skill_schema": {}}]})
    assert resp.status_code == 422  # pydantic min_length=2


def test_fuse_endpoint_returns_structure():
    # 用上下文管理器形式构造 TestClient，覆盖完整 lifespan。
    with TestClient(app) as client:
        resp = client.post("/api/v1/fusion/fuse", json=_payload())
    assert resp.status_code == 200
    data = resp.json()
    assert data["name"] and data["skill_schema"]
    assert data["operator"]["id"]
    assert "degraded" in data

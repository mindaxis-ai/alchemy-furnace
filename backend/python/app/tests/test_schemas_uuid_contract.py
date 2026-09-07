# -*- coding: utf-8 -*-
"""
跨服务身份字段 UUID 契约测试 (011 Task 8 / Step 4)

Go 网关已保证所有跨服务身份字段为 UUID 文本字符串；Python 合成引擎在
schema 层做同款校验作为纵深防御：非 UUID 的身份字段（假 slug、内部
数字 ID、空串等）在入口即被 pydantic 拒绝，不再流入合成与指纹计算。

覆盖：
1. SynthesisPillInput.id：非法值 -> ValidationError（含中文错误说明）；
   合法 UUID 文本 -> 通过
2. 嵌套传播：CombineRequest / FuseRequest 的 pills 复用同一校验

运行：cd backend/python && .venv/bin/pytest app/tests/test_schemas_uuid_contract.py -q
（需真实 pydantic；schemas.py 仅被 app/api/* 导入，桩环境不触及）
"""
import uuid as uuid_mod

import pytest
from pydantic import ValidationError

from app.models.schemas import CombineRequest, FuseRequest, SynthesisPillInput


_VALID_UUID = "550e8400-e29b-41d4-a716-446655440000"


def _pill(**overrides) -> dict:
    """合法金丹入参（id 可被 overrides 覆盖为非法值）"""
    pill = {
        "id": _VALID_UUID,
        "name": "测试金丹",
        "weight": 1.0,
        "sort_order": 0,
        "skill_schema": {"k": "v"},
    }
    pill.update(overrides)
    return pill


class TestSynthesisPillInputId:
    """SynthesisPillInput.id 必须为 UUID 字符串"""

    @pytest.mark.parametrize(
        "bad_id",
        [
            "pill-1",        # 假 slug（旧式 fixture id）
            "12345",         # 内部数字 ID（011 明确禁止回流）
            "",              # 空串
            "not-a-uuid",    # 任意非 UUID 文本
            "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",  # 8-4-4-4-12 但含非 hex 字符
        ],
    )
    def test_invalid_id_rejected(self, bad_id):
        with pytest.raises(ValidationError):
            SynthesisPillInput(**_pill(id=bad_id))

    def test_error_message_states_uuid_contract(self):
        """错误信息中文说明「金丹 id 必须为 UUID 字符串」"""
        with pytest.raises(ValidationError) as exc_info:
            SynthesisPillInput(**_pill(id="pill-1"))
        assert "金丹 id 必须为 UUID 字符串" in str(exc_info.value)

    def test_valid_uuid_accepted(self):
        pill = SynthesisPillInput(**_pill())
        assert pill.id == _VALID_UUID

    def test_generated_uuid4_accepted(self):
        pill_id = str(uuid_mod.uuid4())
        assert SynthesisPillInput(**_pill(id=pill_id)).id == pill_id


class TestNestedRequests:
    """CombineRequest / FuseRequest 的 pills 走同一 UUID 校验"""

    def test_combine_request_rejects_invalid_pill_id(self):
        with pytest.raises(ValidationError):
            CombineRequest(pills=[_pill(id="12345")])

    def test_combine_request_accepts_uuid_pills(self):
        request = CombineRequest(pills=[_pill()])
        assert request.pills[0].id == _VALID_UUID

    def test_fuse_request_rejects_invalid_pill_id(self):
        with pytest.raises(ValidationError):
            FuseRequest(pills=[_pill(id="pill-1"), _pill()])

    def test_fuse_request_accepts_uuid_pills(self):
        request = FuseRequest(pills=[_pill(), _pill()])
        assert request.pills[1].id == _VALID_UUID

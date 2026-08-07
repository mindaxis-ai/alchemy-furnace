# -*- coding: utf-8 -*-
"""
炼丹炉 - 金丹质量校验路由 (Quality API)
- POST /api/v1/quality/validate-pill - 校验 nuwa-skill schema 完整性
"""
import logging
from typing import Any, Dict, List

from fastapi import APIRouter
from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/quality", tags=["质检 - 验丹"])


class ValidatePillRequest(BaseModel):
    """验丹请求"""
    skill_schema: Dict[str, Any] = Field(..., description="nuwa-skill 结构化内容")


class ValidatePillResponse(BaseModel):
    """验丹响应"""
    valid: bool = Field(..., description="是否通过校验")
    score: int = Field(..., ge=0, le=100, description="质量评分")
    issues: List[str] = Field(default_factory=list, description="问题列表")


@router.post(
    "/validate-pill",
    response_model=ValidatePillResponse,
    summary="校验金丹 schema",
    description="检查 nuwa-skill schema 必需字段与结构完整性",
)
async def validate_pill(request: ValidatePillRequest) -> ValidatePillResponse:
    schema = request.skill_schema or {}
    issues: List[str] = []
    score = 100

    # 必需：expression_dna
    dna = schema.get("expression_dna")
    if not isinstance(dna, dict) or not dna:
        issues.append("缺少 expression_dna（表达 DNA 为必需字段）")
        score -= 40
    else:
        if not dna.get("sentence_length"):
            issues.append("expression_dna.sentence_length 缺失")
            score -= 5
        if dna.get("formality") is None:
            issues.append("expression_dna.formality 缺失")
            score -= 5
        elif not isinstance(dna["formality"], (int, float)) or not (0 <= dna["formality"] <= 1):
            issues.append("expression_dna.formality 必须为 0-1 的数值")
            score -= 10

    # 建议：mental_models
    mental_models = schema.get("mental_models")
    if not isinstance(mental_models, list):
        issues.append("mental_models 应为数组")
        score -= 10
    else:
        if len(mental_models) > 20:
            issues.append("mental_models 数量超过 20 个上限")
            score -= 10
        if len(mental_models) < 1:
            issues.append("建议至少包含 1 个 mental_model")
            score -= 5

    # 建议：honest_limits
    if not schema.get("honest_limits"):
        issues.append("缺少 honest_limits（诚实边界）")
        score -= 10

    # 建议：example_dialogues
    dialogues = schema.get("example_dialogues")
    if isinstance(dialogues, list) and len(dialogues) > 10:
        issues.append("example_dialogues 数量超过 10 个上限")
        score -= 5

    # 建议：identity_card
    if not schema.get("identity_card"):
        issues.append("缺少 identity_card（第一人称身份卡）")
        score -= 10

    score = max(0, score)
    valid = score >= 60 and isinstance(dna, dict) and bool(dna)

    return ValidatePillResponse(valid=valid, score=score, issues=issues)

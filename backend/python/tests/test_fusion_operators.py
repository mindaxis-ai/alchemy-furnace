# backend/python/tests/test_fusion_operators.py
# -*- coding: utf-8 -*-
from app.services.fusion_operators import (
    FUSION_OPERATORS, FusionOperator, get_operator, sample_operator,
)


def test_seven_operators_with_unique_ids():
    assert len(FUSION_OPERATORS) == 7
    ids = [op.id for op in FUSION_OPERATORS]
    assert len(set(ids)) == 7


def test_operator_fields_nonempty():
    for op in FUSION_OPERATORS:
        assert isinstance(op, FusionOperator)
        assert op.id and op.name and op.instruction


def test_sample_operator_returns_member():
    op = sample_operator()
    assert op in FUSION_OPERATORS


def test_get_operator_hit_and_miss():
    first = FUSION_OPERATORS[0]
    assert get_operator(first.id) is first
    assert get_operator("nonexistent") is None


def test_sample_distribution_covers_all():
    seen = {sample_operator().id for _ in range(200)}
    assert seen == {op.id for op in FUSION_OPERATORS}

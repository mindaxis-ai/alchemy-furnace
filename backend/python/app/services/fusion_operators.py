# backend/python/app/services/fusion_operators.py
# -*- coding: utf-8 -*-
"""
融合算子库(Promptbreeder 风格) - 金丹融合的变异算子集合

每次融合均匀随机抽 1 个算子注入 LLM prompt,驱动产物多样性。
参考: Promptbreeder (arXiv:2309.16797) mutation operators;
      Blended Skill Talk (arXiv:2004.08449) 多技能融合。
"""
import random
from dataclasses import dataclass
from typing import Optional


@dataclass(frozen=True)
class FusionOperator:
    id: str           # 英文 slug,用于 lineage 记录
    name: str         # 中文名,用于 UI 展示
    instruction: str  # 注入 LLM prompt 的算子指令


FUSION_OPERATORS: list[FusionOperator] = [
    FusionOperator(
        id="hyperbole",
        name="夸张突变",
        instruction=(
            "把每枚原料金丹最鲜明的特质推向极端,合并后风格浓度翻倍。"
            "不要怕过头——融合产物应当比任何一枚原料都更有个性和记忆点。"
        ),
    ),
    FusionOperator(
        id="distillation",
        name="蒸馏提炼",
        instruction=(
            "剥离所有原料金丹的表层装饰,只保留它们最深层的精神共同点。"
            "产物应当像原料们的「公约数」——纯粹、克制、直击本质。"
        ),
    ),
    FusionOperator(
        id="dialectic",
        name="对立调和",
        instruction=(
            "找出原料金丹之间的内在矛盾(风格、立场、气质上的冲突),"
            "不要消除矛盾,而是让矛盾成为新人格的张力引擎——"
            "产物应当是一个「内部有对话感」的人格。"
        ),
    ),
    FusionOperator(
        id="inversion",
        name="角色反转",
        instruction=(
            "故意反转各原料金丹的核心立场或气质,再进行融合。"
            "产物应当是一个「熟悉的陌生人」——能认出原料的影子,但走向了反面。"
        ),
    ),
    FusionOperator(
        id="dilution",
        name="血统稀释",
        instruction=(
            "任选一枚原料金丹作为主导(占约 70% 的人格权重),"
            "其余金丹作为香料点缀。产物应当明显像主导者,但有关键的异域风味。"
        ),
    ),
    FusionOperator(
        id="recombination",
        name="基因重组",
        instruction=(
            "做字段级杂交:从不同原料金丹中分别摘取 vocabulary、rhythm、"
            "mental_models、decision_heuristics 等不同部分重新组合,"
            "像基因重组一样拼出一个全新整体,而不是简单的风格平均。"
        ),
    ),
    FusionOperator(
        id="emergent",
        name="涌现变异",
        instruction=(
            "不要直接混合原料金丹——想象这些人格共同教导出的学生、"
            "或他们合作创办的机构会是什么样子。产物是原料们的「下一代」,"
            "带着他们的影响,但是一个全新的人。"
        ),
    ),
]

_OPERATOR_INDEX = {op.id: op for op in FUSION_OPERATORS}


def sample_operator() -> FusionOperator:
    """均匀随机抽 1 个融合算子"""
    return random.choice(FUSION_OPERATORS)


def get_operator(operator_id: str) -> Optional[FusionOperator]:
    """按 id 取算子(重试时避免重复抽同一算子)"""
    return _OPERATOR_INDEX.get(operator_id)

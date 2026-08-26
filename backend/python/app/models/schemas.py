# -*- coding: utf-8 -*-
"""
炼丹炉 · 金丹化性 - Pydantic 数据模型 (Data Schemas)
定义语言模式合成与对话的请求/响应数据契约
"""
from typing import List, Dict, Any, Optional, Literal
from pydantic import BaseModel, Field


# ==================== 通用响应模型 ====================

class BaseResponse(BaseModel):
    """基础响应模型 - 万法归宗"""
    code: int = Field(default=0, description="状态码：0为成功，非0为失败")
    message: str = Field(default="success", description="响应消息")
    data: Optional[Any] = Field(default=None, description="响应数据")


# ==================== 语言模式合成相关模型 ====================

class ExpressionDNA(BaseModel):
    """表达 DNA - 丹之气象"""
    sentence_length: Optional[str] = Field(default=None, description="句式长度: short/medium/long/mixed")
    formality: Optional[float] = Field(default=None, ge=0.0, le=1.0, description="正式程度 0-1")
    vocabulary: List[str] = Field(default_factory=list, description="高频词")
    taboo_words: List[str] = Field(default_factory=list, description="禁用词")
    rhythm: Optional[str] = Field(default=None, description="节奏")
    humor_type: Optional[str] = Field(default=None, description="幽默类型")
    certainty_style: Optional[str] = Field(default=None, description="确定性表达风格")
    citation_habit: Optional[str] = Field(default=None, description="引用习惯")


class SynthesisPillInput(BaseModel):
    """
    合成请求中的单颗金丹

    Attributes:
        id: 金丹ID（UUID 字符串）
        name: 金丹名称
        weight: 剂量/权重
        sort_order: 服用顺序
        skill_schema: nuwa-skill 结构化内容
    """
    id: str = Field(..., min_length=1, description="金丹ID（UUID 字符串）")
    name: str = Field(..., description="金丹名称")
    weight: float = Field(default=1.0, ge=0.0, le=10.0, description="剂量/权重")
    sort_order: int = Field(default=0, ge=0, description="服用顺序")
    skill_schema: Dict[str, Any] = Field(..., description="nuwa-skill 结构化内容")


class CombineRequest(BaseModel):
    """
    语言模式合成请求 - 化丹为性

    Attributes:
        personality: 道人基础性格描述
        pills: 已服用金丹列表（含权重/顺序）
        model: 用于涌现推导的 LLM 模型
        temperature: 温度参数
        max_tokens: 最大 token 数
    """
    personality: str = Field(default="", description="道人基础性格描述")
    pills: List[SynthesisPillInput] = Field(default_factory=list, description="金丹列表")
    model: str = Field(default="", description="合成用 LLM 模型")
    api_key: Optional[str] = Field(default=None, description="按请求覆盖的 API 密钥（缺省回退环境变量）")
    base_url: Optional[str] = Field(default=None, description="按请求覆盖的 OpenAI 兼容接口地址")
    temperature: float = Field(default=0.7, ge=0.0, le=2.0, description="温度参数")
    max_tokens: int = Field(default=2048, ge=1, le=8192, description="最大 token 数")


class InnerTension(BaseModel):
    """
    内在冲突 - 丹性相冲

    Attributes:
        dimension: 冲突维度（如 sentence_length / formality）
        description: 冲突描述
        severity: 严重程度 low/medium/high
    """
    dimension: str = Field(..., description="冲突维度")
    description: str = Field(..., description="冲突描述")
    severity: Literal["low", "medium", "high"] = Field(default="medium", description="严重程度")


class CombineResponse(BaseModel):
    """
    语言模式合成响应 - 丹性已成

    Attributes:
        system_prompt: 合成后的系统提示词
        emergence_rules: 涌现规则列表
        inner_tensions: 检测到的内在冲突
        fingerprint: 来源指纹（SHA256）
        model: 使用的合成模型
        usage: token 用量
        degraded: 是否走了结构化合并兜底(LLM 不可用/失败);True 时 Go 端不落库
    """
    system_prompt: str = Field(..., description="合成后的系统提示词")
    emergence_rules: List[Any] = Field(default_factory=list, description="涌现规则列表")
    inner_tensions: List[InnerTension] = Field(default_factory=list, description="内在冲突")
    fingerprint: str = Field(..., description="来源指纹 SHA256")
    model: str = Field(default="", description="使用的合成模型")
    usage: Dict[str, int] = Field(default_factory=dict, description="token 用量")
    degraded: bool = Field(default=False, description="是否走了兜底提示词(LLM 不可用/失败)")


# ==================== 女娲蒸馏 ====================

class DistillRequest(BaseModel):
    subject: str = Field(..., min_length=2, max_length=120, description="人物或主题")
    brief: str = Field(..., min_length=4, max_length=1000, description="用户的粗略目标描述")
    model: str = Field(default="", description="蒸馏模型")
    api_key: Optional[str] = Field(default=None, description="按请求覆盖 API Key")
    base_url: Optional[str] = Field(default=None, description="按请求覆盖 OpenAI 兼容地址")
    locale: Literal["zh-CN", "en"] = Field(default="zh-CN")


class DistillSource(BaseModel):
    title: str
    url: str
    dimension: str


class DistillResearchSummary(BaseModel):
    """蒸馏研究摘要 - 证据等级与来源统计(不含正文)"""
    evidence_level: Literal["limited", "standard"] = Field(..., description="证据等级: limited/standard")
    document_count: int = Field(..., ge=0, description="采纳的公开资料文档数")
    domain_count: int = Field(..., ge=0, description="独立域名数")
    total_characters: int = Field(..., ge=0, description="证据总字符数")
    warnings: List[str] = Field(default_factory=list, description="研究阶段警告")


class DistillResponse(BaseModel):
    name: str
    description: str
    persona_summary: str
    tags: List[str] = Field(default_factory=list)
    skill_schema: Dict[str, Any]
    sources: List[DistillSource] = Field(default_factory=list)
    model: str = ""
    research: DistillResearchSummary


# ==================== 对话相关模型 ====================

class ChatMessage(BaseModel):
    """
    对话消息 - 道人与修士的问答

    Attributes:
        role: 角色 (system/user/assistant)
        content: 消息内容
    """
    role: Literal["system", "user", "assistant"] = Field(..., description="角色")
    content: str = Field(..., description="消息内容")


class ChatCompletionRequest(BaseModel):
    """
    对话请求 - 求道之问

    Attributes:
        messages: 消息历史（应已包含合成后的 system 消息）
        model: 使用的LLM模型
        temperature: 温度参数
        max_tokens: 最大token数
        stream: 是否流式返回
    """
    messages: List[ChatMessage] = Field(..., min_length=1, description="消息历史")
    model: str = Field(default="", description="LLM模型")
    api_key: Optional[str] = Field(default=None, description="按请求覆盖的 API 密钥（缺省回退环境变量）")
    base_url: Optional[str] = Field(default=None, description="按请求覆盖的 OpenAI 兼容接口地址")
    temperature: float = Field(default=0.7, ge=0.0, le=2.0, description="温度参数")
    max_tokens: int = Field(default=4096, ge=1, le=8192, description="最大token数")
    stream: bool = Field(default=False, description="是否流式返回")


class ChatCompletionResponse(BaseModel):
    """
    对话响应 - 道人应答

    Attributes:
        content: AI生成的回答
        model: 使用的模型
        usage: token用量
    """
    content: str = Field(..., description="AI回答")
    model: str = Field(default="", description="使用模型")
    usage: Dict[str, int] = Field(default_factory=dict, description="token用量")


# ==================== 系统相关模型 ====================

class HealthResponse(BaseModel):
    """健康检查响应 - 探查炼丹炉状态"""
    status: str = Field(default="ok", description="服务状态")
    version: str = Field(default="2.0.0", description="版本号")
    components: Dict[str, str] = Field(
        default_factory=dict, description="各组件状态"
    )


class ErrorResponse(BaseModel):
    """错误响应 - 炼丹失败之因"""
    code: int = Field(..., description="错误码")
    message: str = Field(..., description="错误消息")
    detail: Optional[str] = Field(default=None, description="详细错误信息")


# ==================== 金丹融合相关模型 ====================


class FuseRequest(BaseModel):
    """金丹融合请求 - 合丹为新"""
    pills: List[SynthesisPillInput] = Field(..., min_length=2, description="原料金丹(至少 2 枚)")
    model: str = Field(default="", description="融合用 LLM 模型(空则回退默认)")
    api_key: Optional[str] = Field(default=None, description="按请求覆盖的 API 密钥")
    base_url: Optional[str] = Field(default=None, description="按请求覆盖的 OpenAI 兼容接口地址")
    exclude_operator_id: Optional[str] = Field(default=None, description="重试时要排除的算子 id")


class FuseOperatorInfo(BaseModel):
    id: str
    name: str


class FuseResponse(BaseModel):
    """金丹融合响应"""
    name: str
    description: str
    skill_schema: Dict[str, Any]
    operator: FuseOperatorInfo
    model: str = ""
    degraded: bool = Field(default=False, description="是否走了保底方案(LLM 连续失败)")

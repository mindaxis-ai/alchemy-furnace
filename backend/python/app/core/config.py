# -*- coding: utf-8 -*-
"""
炼丹炉 · 金丹化性 - 配置管理模块 (Configuration)
以 pydantic_settings 管理环境变量，犹如炼丹之天时地利
"""
from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field


class Settings(BaseSettings):
    """
    炼丹炉全局配置 - 天地法则

    所有配置项均可通过环境变量覆盖，环境变量名大写。
    例如: OPENAI_API_KEY, SYNTHESIS_MODEL 等。
    """

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
    )

    # ==================== 服务配置 ====================
    app_name: str = Field(default="炼丹炉 · 语言引擎", description="应用名称")
    app_version: str = Field(default="2.0.0", description="应用版本")
    debug: bool = Field(default=False, description="调试模式")

    # ==================== 服务器配置 ====================
    host: str = Field(default="0.0.0.0", description="监听地址")
    port: int = Field(default=8000, description="监听端口")

    # ==================== CORS 配置 ====================
    cors_origins: list[str] = Field(
        default=["*"], description="允许的跨域来源"
    )

    # ==================== LLM 配置 ====================
    openai_api_key: str = Field(default="", description="OpenAI API 密钥")
    openai_base_url: str = Field(
        default="https://api.openai.com/v1", description="OpenAI 兼容接口地址"
    )
    default_model: str = Field(
        default="gpt-4o", description="默认 LLM 模型"
    )
    synthesis_model: str = Field(
        default="gpt-4o-mini", description="语言模式合成用模型（可用较小模型）"
    )

    # ==================== 日志配置 ====================
    log_level: str = Field(default="INFO", description="日志级别")
    log_format: str = Field(
        default="%(asctime)s [%(levelname)s] %(name)s - %(message)s",
        description="日志格式"
    )

    @property
    def openai_api_key_valid(self) -> bool:
        """检查 OpenAI API 密钥是否有效配置"""
        return bool(self.openai_api_key and self.openai_api_key.startswith("sk-"))


# ==================== 全局配置实例 ====================
settings = Settings()

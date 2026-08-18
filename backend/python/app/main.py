# -*- coding: utf-8 -*-
"""
================================================================================
炼丹炉 · 语言引擎 - FastAPI 入口
================================================================================
金丹 = 语言模式/人格特质技能包，道人 = AI Agent，化丹为性 = 语言模式合成

启动命令:
    uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

API 文档:
    启动后访问 http://localhost:8000/docs (Swagger UI)
    或 http://localhost:8000/redoc (ReDoc)
================================================================================
"""
import logging
import sys

from fastapi import FastAPI, status
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from contextlib import asynccontextmanager

from app.core.config import settings

# ==================== 日志配置 ====================

logging.basicConfig(
    level=getattr(logging, settings.log_level.upper(), logging.INFO),
    format=settings.log_format,
    handlers=[
        logging.StreamHandler(sys.stdout),
    ],
)
logger = logging.getLogger("alchemy-furnace")


# ==================== 生命周期管理 ====================

@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理 - 炼丹炉启停"""
    logger.info("=" * 60)
    logger.info("炼丹炉 · 语言引擎启动中...")
    logger.info(f"版本: {settings.app_version}")
    logger.info(f"环境: {'调试' if settings.debug else '生产'}")
    logger.info("=" * 60)

    if not settings.openai_api_key_valid:
        logger.warning("未配置有效的 OPENAI_API_KEY，合成与对话功能将不可用")

    logger.info("炼丹炉启动完毕，开始接客！")

    yield  # 应用运行期间

    logger.info("炼丹炉正在关闭...")
    logger.info("炼丹炉已安全关闭")


# ==================== FastAPI 应用实例 ====================

app = FastAPI(
    title=settings.app_name,
    description="""
    炼丹炉 · 语言引擎 - 金丹化性（Skill-Persona Alchemy）系统。

    ## 核心概念
    - **金丹**: 语言模式/人格特质技能包（nuwa-skill 结构）
    - **道人**: AI Agent（基础性格 + 已服用金丹）
    - **化丹为性**: 语言模式合成（结构化合并 + LLM 涌现推导）

    ## 功能模块
    - 语言模式合成：POST /api/v1/synthesis/combine
    - 对话：POST /api/v1/chat/completions（非流式）、/stream（SSE 流式）
    - 金丹质检：POST /api/v1/quality/validate-pill
    """,
    version=settings.app_version,
    docs_url="/docs",
    redoc_url="/redoc",
    openapi_url="/openapi.json",
    lifespan=lifespan,
)

# ==================== CORS 配置 ====================

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# ==================== 全局异常处理 ====================

@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    """全局异常处理 - 炼丹出错之应对"""
    logger.error(f"炼丹出错: {exc}", exc_info=True)
    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={
            "code": -1,
            "message": "炼丹炉内部出错",
            "detail": str(exc) if settings.debug else "请稍后重试",
        },
    )


# ==================== 路由注册 ====================

from app.api import chat, synthesis, quality, fusion

app.include_router(synthesis.router, prefix="/api/v1")
app.include_router(chat.router, prefix="/api/v1")
app.include_router(quality.router, prefix="/api/v1")
app.include_router(fusion.router, prefix="/api/v1")

# ==================== 根路由 ====================

@app.get("/", tags=["根路径"])
async def root():
    """根路径 - 炼丹炉总览"""
    return {
        "name": settings.app_name,
        "version": settings.app_version,
        "description": "炼丹炉 · 语言引擎 - 金丹化性系统",
        "docs": "/docs",
        "endpoints": {
            "synthesis": "/api/v1/synthesis",
            "chat": "/api/v1/chat",
            "quality": "/api/v1/quality",
        },
    }


@app.get("/health", tags=["系统"])
@app.get("/api/v1/health", tags=["系统"])
async def health_check():
    """
    健康检查 - 探查炼丹炉状态

    按内部契约返回组件状态：
    - openai: ok / not_configured（依据 OPENAI_API_KEY 是否配置）
    - database: not_applicable（语言引擎不直连数据库，数据归 Go 网关管理）
    """
    components = {
        "openai": "ok" if settings.openai_api_key_valid else "not_configured",
        "database": "not_applicable",
    }

    overall = "ok" if components["openai"] == "ok" else "degraded"

    return {
        "status": overall,
        "version": settings.app_version,
        "components": components,
    }


# ==================== 启动入口 ====================

if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "app.main:app",
        host=settings.host,
        port=settings.port,
        reload=settings.debug,
        log_level=settings.log_level.lower(),
    )

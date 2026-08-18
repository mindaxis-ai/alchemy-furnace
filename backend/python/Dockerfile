# ═══════════════════════════════════════════
# Python 语言模式合成引擎 - 炼丹核心
#
# 多阶段构建：
#   builder —— 安装 build-essential + Python 依赖（pip cache mount）
#   runtime —— slim 镜像 + 非 root 用户 + 复制 site-packages
#
# 受限网络传代理：docker build --build-arg HTTPS_PROXY=http://host:port .
# ═══════════════════════════════════════════

# ---- builder ----
FROM python:3.11-slim AS builder

ARG HTTP_PROXY
ARG HTTPS_PROXY
ENV HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    PIP_DISABLE_PIP_VERSION_CHECK=1

# 仅构建期需要：gcc / make（某些 pip 包要编译）
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 依赖层优先
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --prefix=/install -r requirements.txt

# ---- runtime ----
FROM python:3.11-slim

ARG HTTP_PROXY
ARG HTTPS_PROXY
ENV HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# 运行时需要：ffmpeg（可选音频处理） + curl（健康检查） + ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# 非 root（debian 风格：python:3.11-slim 是 debian,不是 alpine）
RUN groupadd -r app && useradd -r -g app app

# 把 builder 装好的 site-packages 复制到 runtime
COPY --from=builder /install /usr/local

WORKDIR /app

# 源码：改 app/ 任意文件只重 build 这一层
COPY --chown=app:app app/ ./app/

RUN mkdir -p /app/uploads && chown -R app:app /app
USER app

EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
    CMD curl -f http://localhost:8000/health || exit 1

STOPSIGNAL SIGTERM

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]

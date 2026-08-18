# ═══════════════════════════════════════════
# Next.js 前端 - 炼丹炉界面
#
# 构建产物 out/（next build 静态导出）复制到共享卷，由 nginx 服务
#
# 受限网络传代理：docker build --build-arg HTTPS_PROXY=http://host:port .
# ═══════════════════════════════════════════

# ---- builder ----
FROM node:20-alpine AS builder

ARG HTTP_PROXY
ARG HTTPS_PROXY
ENV HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    CI=true \
    NEXT_TELEMETRY_DISABLED=1

WORKDIR /app

# 启用 pnpm
RUN corepack enable

# 依赖层优先：package.json/pnpm-lock.yaml 未变直接命中
COPY package.json pnpm-lock.yaml .npmrc* ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# 源码：改 app/ 或 components/ 只重 build 这一层
COPY . .

ARG NEXT_PUBLIC_API_BASE_URL
ARG NEXT_PUBLIC_WS_BASE_URL
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL \
    NEXT_PUBLIC_WS_BASE_URL=$NEXT_PUBLIC_WS_BASE_URL

RUN pnpm build

# ---- runtime ----
# 静态导出：构建器把 out/ 落到挂载卷，由 nginx 服务
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/out ./dist
VOLUME /app/dist
CMD ["sh", "-c", "cp -r /app/dist/* /out/ 2>/dev/null || true"]

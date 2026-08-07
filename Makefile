# ═══════════════════════════════════════════
# 炼丹炉 · 金丹化性系统 - Makefile
# Alchemy Furnace Skill-Persona System
# ═══════════════════════════════════════════

.PHONY: help init build up down logs ps clean dev-front dev-go dev-python test

# 默认目标
help:
	@echo ""
	@echo "  █████╗ ██╗      ██████╗██╗  ██╗███████╗███╗   ███╗██╗   ██╗"
	@echo " ██╔══██╗██║     ██╔════╝██║  ██║██╔════╝████╗ ████║╚██╗ ██╔╝"
	@echo " ███████║██║     ██║     ███████║█████╗  ██╔████╔██║ ╚████╔╝ "
	@echo " ██╔══██║██║     ██║     ██╔══██║██╔══╝  ██║╚██╔╝██║  ╚██╔╝  "
	@echo " ██║  ██║███████╗╚██████╗██║  ██║███████╗██║ ╚═╝ ██║   ██║   "
	@echo " ╚═╝  ╚═╝╚══════╝ ╚═════╝╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝   ╚═╝   "
	@echo ""
	@echo "  🏺 炼丹炉 · 金丹化性系统 - 可用命令"
	@echo ""
	@echo "  系统管理:"
	@echo "    make init         初始化环境（复制 .env 文件）"
	@echo "    make build        构建所有 Docker 镜像"
	@echo "    make up           启动所有服务（后台运行）"
	@echo "    make down         停止所有服务"
	@echo "    make restart      重启所有服务"
	@echo "    make logs         查看所有服务日志"
	@echo "    make ps           查看服务状态"
	@echo "    make clean        清理所有数据和镜像"
	@echo ""
	@echo "  独立开发:"
	@echo "    make dev-front    启动前端开发服务器"
	@echo "    make dev-go       启动 Go 后端开发服务器"
	@echo "    make dev-python   启动 Python 语言引擎开发服务器"
	@echo ""
	@echo "  测试与维护:"
	@echo "    make test         运行所有测试"
	@echo "    make migrate      执行数据库迁移"
	@echo "    make seed         写入内置示例金丹"
	@echo "    make format       格式化所有代码"
	@echo ""

# ─── 系统管理 ───

init:
	@echo "⚗️  正在初始化炼丹炉环境..."
	@if [ ! -f .env ]; then cp .env.example .env; echo "✅ 已创建 .env 文件，请编辑配置"; else echo "⚠️  .env 文件已存在"; fi
	@echo "✅ 初始化完成，请编辑 .env 文件配置 API Key"

build:
	@echo "🏗️  正在构建炼丹炉..."
	docker-compose build
	@echo "✅ 构建完成"

up:
	@echo "🔥 启动炼丹炉..."
	docker-compose up -d
	@echo "✅ 炼丹炉已启动"
	@echo "   前端: http://localhost"
	@echo "   Go API: http://localhost:8080"
	@echo "   Python 语言引擎: http://localhost:8000"

down:
	@echo "🛑 停止炼丹炉..."
	docker-compose down
	@echo "✅ 已停止"

restart: down up

logs:
	docker-compose logs -f

logs-go:
	docker-compose logs -f go-api

logs-python:
	docker-compose logs -f python-engine

logs-db:
	docker-compose logs -f postgres

ps:
	docker-compose ps

clean:
	@echo "🧹 清理炼丹炉..."
	docker-compose down -v
	docker system prune -f
	@echo "✅ 清理完成"

# ─── 独立开发 ───

dev-front:
	@echo "⚡ 启动前端开发服务器..."
	cd frontend && npm install && npm run dev

dev-go:
	@echo "⚡ 启动 Go 后端开发服务器..."
	cd backend/go && go mod tidy && go run cmd/server/main.go

dev-python:
	@echo "⚡ 启动 Python 语言引擎开发服务器..."
	cd backend/python && pip install -r requirements.txt && uvicorn app.main:app --reload

# ─── 测试与维护 ───

test:
	@echo "🧪 运行测试..."
	cd backend/go && go test ./...
	cd backend/python && pytest

test-go:
	@echo "🧪 运行 Go 测试..."
	cd backend/go && go test ./...

test-python:
	@echo "🧪 运行 Python 测试..."
	cd backend/python && pytest

test-frontend:
	@echo "🧪 运行前端类型检查与构建..."
	cd frontend && npm run build

migrate:
	@echo "🗃️  执行数据库迁移..."
	cd backend/go && go run cmd/server/main.go migrate

seed:
	@echo "💊 写入内置示例金丹..."
	cd backend/go && go run cmd/server/main.go seed

format:
	@echo "🎨 格式化代码..."
	cd backend/go && gofmt -w .
	cd frontend && npm run lint

# ─── 一键部署 ───

deploy: init build up
	@echo "🚀 炼丹炉部署完成！"
	@echo "   访问 http://localhost 开始使用"

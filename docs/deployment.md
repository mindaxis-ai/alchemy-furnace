# 部署指南

## 环境要求

- Docker 20.10+
- Docker Compose 2.0+
- 至少 4GB 可用内存
- 至少 10GB 磁盘空间

## 快速部署

### 1. 克隆项目

```bash
git clone https://github.com/yusanwen-code/alchemy-furnace.git
cd alchemy-furnace
```

### 2. 配置环境变量

```bash
# 复制环境变量模板
cp .env.example .env

# 编辑 .env 文件
vim .env
```

至少配置以下项：

```env
# LLM API Key（必填）
OPENAI_API_KEY=sk-your-api-key-here

# 如果使用非 OpenAI 的兼容接口（DeepSeek、通义千问等），修改 base URL
# OPENAI_BASE_URL=https://api.deepseek.com/v1
```

### 3. 一键启动

```bash
make deploy
```

或手动执行：

```bash
docker-compose build
docker-compose up -d
```

### 4. 访问服务

| 服务 | 地址 | 说明 |
|------|------|------|
| 前端界面 | http://localhost | 主要用户界面 |
| Go API | http://localhost:8080 | API 网关 |
| Python 语言引擎 | http://localhost:8000 | 语言模式合成 + LLM 对话，Swagger 文档 |

## 服务组成

`docker-compose.yml` 定义了以下服务：

| 服务 | 镜像/构建 | 说明 | 端口 |
|------|-----------|------|------|
| `postgres` | postgres:14-alpine | 业务数据库 | 5432 |
| `python-engine` | ./backend/python | 语言模式合成引擎 + LLM 调用 | 8000 |
| `go-api` | ./backend/go | API 网关（REST + WebSocket） | 8080 |
| `nginx` | nginx:alpine | 反向代理 + 前端静态文件 | 80 |
| `frontend-builder` | ./frontend (builder target) | 构建前端静态文件至 `frontend_dist` 卷（`build` profile） | - |

数据卷：`postgres_data`（数据库）、`frontend_dist`（前端构建产物）。

## 环境变量

完整列表见 `.env.example`：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_HOST` | postgres | PostgreSQL 主机 |
| `DB_PORT` | 5432 | PostgreSQL 端口 |
| `DB_USER` | alchemy | 数据库用户 |
| `DB_PASSWORD` | alchemy123 | 数据库密码 |
| `DB_NAME` | alchemy_db | 数据库名 |
| `OPENAI_API_KEY` | - | LLM API Key（必填） |
| `OPENAI_BASE_URL` | https://api.openai.com/v1 | OpenAI 兼容接口地址 |
| `DEFAULT_MODEL` | gpt-4o | 默认对话模型 |
| `SYNTHESIS_MODEL` | gpt-4o-mini | 金丹合成/涌现推导所用模型（可用较小模型节省成本） |
| `GO_PORT` | 8080 | Go API 端口 |
| `PYTHON_ENGINE_BASE_URL` | http://python-engine:8000 | Python 语言引擎地址（Go 调用） |
| `PYTHON_PORT` | 8000 | Python 服务端口 |
| `NGINX_PORT` | 80 | Nginx 端口 |

## 开发环境部署

### 方式一：Docker Compose（推荐）

```bash
# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f go-api
docker-compose logs -f python-engine
```

### 方式二：本地开发（热重载）

需要本地安装：Node.js 20+、Go 1.21+、Python 3.11+、PostgreSQL 14

**终端 1 - 前端**：
```bash
cd frontend
npm install
npm run dev
```

**终端 2 - Go 后端**：
```bash
cd backend/go
go mod tidy
go run cmd/server/main.go
```

**终端 3 - Python 语言引擎**：
```bash
cd backend/python
pip install -r requirements.txt
uvicorn app.main:app --reload
```

**终端 4 - 基础设施**：
```bash
# 启动 PostgreSQL
docker-compose up -d postgres
```

## 生产环境部署

### 1. 使用 Nginx 反向代理

已配置在 `infra/nginx/nginx.conf`，Docker Compose 会自动启动 Nginx 容器。

如需使用外部 Nginx，修改配置：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # 前端静态文件
    location / {
        root /path/to/alchemy-furnace/frontend/dist;
        try_files $uri $uri/ /index.html;
    }

    # API 代理（含 WebSocket 升级头）
    location /api/v1/ {
        proxy_pass http://localhost:8080/api/v1/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 2. HTTPS 配置

使用 Let's Encrypt：

```bash
# 安装 certbot
sudo apt install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com
```

### 3. 系统服务

创建 systemd 服务文件 `/etc/systemd/system/alchemy-furnace.service`：

```ini
[Unit]
Description=Alchemy Furnace Skill-Persona System
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/path/to/alchemy-furnace
ExecStart=/usr/bin/docker-compose up -d
ExecStop=/usr/bin/docker-compose down

[Install]
WantedBy=multi-user.target
```

启用服务：

```bash
sudo systemctl enable alchemy-furnace
sudo systemctl start alchemy-furnace
```

## 更新部署

```bash
# 拉取最新代码
git pull origin main

# 重建并重启
docker-compose down
docker-compose pull
docker-compose build --no-cache
docker-compose up -d

# 查看状态
docker-compose ps
```

## 数据备份

### 备份 PostgreSQL

```bash
# 备份
docker exec alchemy-postgres pg_dump -U alchemy alchemy_db > backup.sql

# 恢复
cat backup.sql | docker exec -i alchemy-postgres psql -U alchemy alchemy_db
```

## 故障排查

### 服务无法启动

```bash
# 查看日志
docker-compose logs

# 检查端口占用
lsof -i :8080
lsof -i :8000
lsof -i :5432
```

### 数据库连接失败

```bash
# 检查 PostgreSQL 状态
docker-compose ps postgres

# 进入 PostgreSQL 容器
docker exec -it alchemy-postgres psql -U alchemy -d alchemy_db

# 检查连接配置
cat .env | grep DB_
```

### LLM 调用失败 / 合成失败

```bash
# 检查 Python 语言引擎日志
docker-compose logs python-engine

# 测试 API 连通性
curl http://localhost:8000/health

# 检查 API Key 配置
cat .env | grep OPENAI
```

### 语言模式缓存异常

若道人言谈与所服金丹不符，通常是缓存未失效。道人性格、服用记录或金丹内容变更后缓存应自动失效重建；如怀疑缓存残留，可直接删除 `language_patterns` 表中对应记录，下次对话会自动重新合成。

## 性能优化

### 1. 数据库优化

```sql
-- 索引已随迁移自动创建
CREATE INDEX idx_agent_pills_agent_id ON agent_pills(agent_id);
CREATE INDEX idx_agent_pills_pill_id ON agent_pills(pill_id);
CREATE INDEX idx_chat_messages_session_id ON chat_messages(session_id);
CREATE INDEX idx_language_patterns_agent_id ON language_patterns(agent_id);
```

### 2. 合成成本优化

- 语言模式缓存避免了每次对话重复合成；仅在性格/金丹变化时重建
- 使用较小的 `SYNTHESIS_MODEL`（如 gpt-4o-mini）完成合成，主对话使用用户指定模型

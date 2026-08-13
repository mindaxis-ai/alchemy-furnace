-- 「炼丹炉」初始 schema(006 UUID 双标识形态)
-- 8 张表;对外资源带 uuid 列(PG 原生 uuid 类型,gen_random_uuid() 默认,唯一索引)
-- 内部主键 id BIGSERIAL 仅作库内联结,永不对外暴露

-- 金丹(语言模式/人格特质技能包)
CREATE TABLE elixir_pills (
    id          BIGSERIAL PRIMARY KEY,
    uuid        UUID NOT NULL DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    skill_schema JSONB NOT NULL,
    tags        JSONB,
    author      VARCHAR(100),
    version     VARCHAR(20) DEFAULT '1.0.0',
    is_builtin  BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_elixir_pills_uuid ON elixir_pills(uuid);
CREATE INDEX idx_elixir_pills_is_builtin ON elixir_pills(is_builtin);

-- 道人(AI Agent)
CREATE TABLE dao_agents (
    id          BIGSERIAL PRIMARY KEY,
    uuid        UUID NOT NULL DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    avatar      VARCHAR(255),
    personality TEXT,
    model_name  VARCHAR(50) DEFAULT 'gpt-4o',
    status      VARCHAR(20) DEFAULT 'active',
    proactivity INTEGER DEFAULT 50,
    created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_dao_agents_uuid ON dao_agents(uuid);

-- 服用记录(道人与金丹绑定;无 uuid,不直接对外寻址)
CREATE TABLE agent_pills (
    id          BIGSERIAL PRIMARY KEY,
    agent_id    BIGINT NOT NULL REFERENCES dao_agents(id) ON DELETE CASCADE,
    pill_id     BIGINT NOT NULL REFERENCES elixir_pills(id) ON DELETE CASCADE,
    weight      DOUBLE PRECISION DEFAULT 1.0,
    sort_order  INTEGER DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_agent_pill ON agent_pills(agent_id, pill_id);
CREATE INDEX idx_agent_pills_agent_id ON agent_pills(agent_id);
CREATE INDEX idx_agent_pills_pill_id ON agent_pills(pill_id);

-- 语言模式缓存(纯内部,无 uuid)
CREATE TABLE language_patterns (
    id                BIGSERIAL PRIMARY KEY,
    agent_id          BIGINT NOT NULL REFERENCES dao_agents(id) ON DELETE CASCADE,
    system_prompt     TEXT NOT NULL,
    emergence_rules   JSONB,
    inner_tensions    JSONB,
    source_fingerprint VARCHAR(80) NOT NULL,
    is_valid          BOOLEAN DEFAULT TRUE,
    created_at        TIMESTAMPTZ DEFAULT now(),
    updated_at        TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_language_patterns_agent_id ON language_patterns(agent_id);

-- 对话会话
CREATE TABLE chat_sessions (
    id          BIGSERIAL PRIMARY KEY,
    uuid        UUID NOT NULL DEFAULT gen_random_uuid(),
    type        VARCHAR(10) DEFAULT 'single',
    agent_id    BIGINT REFERENCES dao_agents(id) ON DELETE CASCADE,
    title       VARCHAR(200),
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_chat_sessions_uuid ON chat_sessions(uuid);
CREATE INDEX idx_chat_sessions_agent_id ON chat_sessions(agent_id);
CREATE INDEX idx_chat_sessions_type ON chat_sessions(type);

-- 对话消息(uuid 供消息历史响应安全序列化)
CREATE TABLE chat_messages (
    id          BIGSERIAL PRIMARY KEY,
    uuid        UUID NOT NULL DEFAULT gen_random_uuid(),
    session_id  BIGINT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    role        VARCHAR(20) NOT NULL,
    content     TEXT NOT NULL,
    sources     JSONB,
    agent_id    BIGINT REFERENCES dao_agents(id) ON DELETE SET NULL,
    mentions    JSONB,
    created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_chat_messages_uuid ON chat_messages(uuid);
CREATE INDEX idx_chat_messages_session_id ON chat_messages(session_id);
CREATE INDEX idx_chat_messages_agent_id ON chat_messages(agent_id);

-- 群聊成员(仅 group 会话)
CREATE TABLE session_members (
    id          BIGSERIAL PRIMARY KEY,
    session_id  BIGINT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    agent_id    BIGINT NOT NULL REFERENCES dao_agents(id) ON DELETE CASCADE,
    sort_order  INTEGER DEFAULT 0,
    joined_at   TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_session_agent ON session_members(session_id, agent_id);
CREATE INDEX idx_session_members_session_id ON session_members(session_id);

-- LLM 供应商配置
CREATE TABLE llm_providers (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID NOT NULL DEFAULT gen_random_uuid(),
    name              VARCHAR(50) NOT NULL,
    display_name      VARCHAR(100) NOT NULL,
    protocol          VARCHAR(50) DEFAULT 'openai-compatible',
    base_url          VARCHAR(255) NOT NULL,
    api_key_encrypted TEXT,
    is_enabled        BOOLEAN DEFAULT TRUE,
    sort_order        INTEGER DEFAULT 0,
    remark            VARCHAR(255) DEFAULT '',
    created_at        TIMESTAMPTZ DEFAULT now(),
    updated_at        TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_llm_providers_uuid ON llm_providers(uuid);
CREATE UNIQUE INDEX idx_llm_providers_name ON llm_providers(name);
CREATE INDEX idx_llm_providers_is_enabled ON llm_providers(is_enabled);

-- LLM 模型配置
CREATE TABLE llm_models (
    id           BIGSERIAL PRIMARY KEY,
    uuid         UUID NOT NULL DEFAULT gen_random_uuid(),
    provider_id  BIGINT NOT NULL REFERENCES llm_providers(id),
    name         VARCHAR(100) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    temperature  DOUBLE PRECISION DEFAULT 0.7,
    max_tokens   INTEGER DEFAULT 4096,
    is_enabled   BOOLEAN DEFAULT TRUE,
    is_default   BOOLEAN DEFAULT FALSE,
    is_synthesis BOOLEAN DEFAULT FALSE,
    is_fusion      BOOLEAN DEFAULT FALSE,
    sort_order   INTEGER DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX idx_llm_models_uuid ON llm_models(uuid);
CREATE UNIQUE INDEX idx_llm_models_provider_name ON llm_models(provider_id, name);
CREATE INDEX idx_llm_models_provider_id ON llm_models(provider_id);
CREATE INDEX idx_llm_models_is_enabled ON llm_models(is_enabled);
-- 部分唯一索引: 全表至多一个默认模型 / 一个合成专用模型 / 一个融合专用模型
CREATE UNIQUE INDEX idx_llm_models_default ON llm_models(is_default) WHERE is_default;
CREATE UNIQUE INDEX idx_llm_models_synthesis ON llm_models(is_synthesis) WHERE is_synthesis;
CREATE UNIQUE INDEX idx_llm_models_fusion ON llm_models(is_fusion) WHERE is_fusion;

# WebSocket Contract: Streaming Chat

**Endpoint**: `WS /api/v1/chat/:session_id/stream`
**Transport**: WebSocket
**Message Format**: JSON

---

## Connection

1. Client opens WebSocket to `/api/v1/chat/:session_id/stream`.
2. Server validates `session_id` exists.
3. Server loads associated `DaoAgent` and its `LanguagePattern` cache.
4. If cache is missing or invalid, server calls Python synthesis service to build/refresh it.
5. Server streams LLM response chunks to client.

## Client → Server Messages

### Send User Message

```json
{
  "type": "chat",
  "content": "你好，清风道长"
}
```

**Fields**:
- `type`: `chat`
- `content`: user message text (required, non-empty)

### Ping

```json
{
  "type": "ping"
}
```

Server responds with:

```json
{
  "type": "pong"
}
```

## Server → Client Messages

### Start

```json
{
  "type": "start",
  "message_id": 123,
  "model": "gpt-4o"
}
```

### Content Chunk

```json
{
  "type": "chunk",
  "content": "你",
  "message_id": 123
}
```

### Done

```json
{
  "type": "done",
  "message_id": 123,
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "total_tokens": 150
  }
}
```

### Error

```json
{
  "type": "error",
  "error": "调用语言引擎失败",
  "detail": "..."
}
```

## Message Flow

```text
Client                              Server
  |                                   |
  |------ WS /chat/1/stream -------->|
  |                                   | load session + agent + pattern
  |<---- {type:"start"} --------------|
  |------ {type:"chat", content:"hi"}->|
  |                                   | save user msg
  |                                   | call Python stream
  |<---- {type:"chunk", content:"你"}--|
  |<---- {type:"chunk", content:"好"}--|
  |<---- {type:"done", usage:{...}}---|
  |                                   | save assistant msg
```

## Notes

- No `sources` field is sent (RAG removed).
- `message_id` refers to the assistant message that will be persisted after the stream completes.
- If the synthesis cache is stale, the server may send an intermediate `{type:"synthesizing"}` message before `start`.

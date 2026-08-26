/**
 * 会话页面地址的唯一事实来源。
 *
 * 桌面端使用 Next output:export，动态路由 `/chat/[sessionId]` 只会预渲染占位
 * `_.html`，真实 UUID 无法通过 useParams() 恢复。因此会话地址统一为查询参数：
 *   大厅: /chat
 *   会话: /chat?session=<UUID>
 * 所有入口都必须用这里的函数生成/解析，禁止散落 `/chat/${id}` 模板字符串。
 */

export const CHAT_SESSION_QUERY_KEY = 'session'

// RFC 4122 通用 UUID（含 version/variant 位），大小写不敏感
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

type SearchParamsReader = Pick<URLSearchParams, 'get'>

/** 论道大厅地址 */
export function chatLobbyHref(): '/chat' {
  return '/chat'
}

/** 单个会话的规范地址；非法 id 直接抛错，避免把占位/脏值拼成可访问链接 */
export function chatSessionHref(sessionId: string): string {
  if (!UUID_RE.test(sessionId)) throw new Error('Invalid chat session id')
  return `/chat?${CHAT_SESSION_QUERY_KEY}=${encodeURIComponent(sessionId)}`
}

/** 从查询参数解析出合法的会话 UUID；缺失/占位/畸形一律返回 undefined */
export function parseChatSessionId(searchParams: SearchParamsReader): string | undefined {
  const value = searchParams.get(CHAT_SESSION_QUERY_KEY)?.trim()
  return value && UUID_RE.test(value) ? value : undefined
}

/**
 * 识别历史动态路径 `/chat/<UUID>`，仅用于一次性兼容重定向。
 * 不匹配大厅、其它实体路径、占位 `_` 或非法 UUID。
 */
export function parseLegacyChatPath(pathname: string): string | undefined {
  const match = pathname.match(/^\/chat\/([^/]+)\/?$/)
  if (!match) return undefined
  const value = decodeURIComponent(match[1])
  return UUID_RE.test(value) ? value : undefined
}

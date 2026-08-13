/**
 * API 基础封装层
 * 基于 fetch 的统一请求封装，自动解包后端 { code, message, data } 响应信封
 */

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL || '/api/v1'

/**
 * 桌面端 token: Wails 重定向 URL 注入一次,存 sessionStorage 后清除 URL 痕迹
 * 桌面模式: cmd/desktop 随机生成 32B token,webview 重定向时挂到 ?token=
 * web 模式: 无 token 注入,authHeaders() 返回空,行为零变化
 */
const DESKTOP_TOKEN_KEY = 'alchemy_desktop_token'

function initDesktopToken(): void {
  if (typeof window === 'undefined') return
  const url = new URL(window.location.href)
  const token = url.searchParams.get('token')
  if (token) {
    sessionStorage.setItem(DESKTOP_TOKEN_KEY, token)
    url.searchParams.delete('token')
    window.history.replaceState(null, '', url.toString())
  }
}
initDesktopToken()

/**
 * 桌面端 API 防护头;web 模式(无 token)返回空对象,零侵入
 * 出口: api.request() 与 chatService.streamChatMessage 的 fetch 均合并此头
 */
export function authHeaders(): Record<string, string> {
  if (typeof window === 'undefined') return {}
  const token = sessionStorage.getItem(DESKTOP_TOKEN_KEY)
  return token ? { 'X-Alchemy-Token': token } : {}
}

/**
 * 统一 API 响应信封
 */
export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
}

/**
 * 请求选项
 */
interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>
}

/**
 * 构建完整 URL（带查询参数）
 */
function buildUrl(path: string, params?: Record<string, string | number | boolean | undefined>): string {
  const url = new URL(API_BASE + path, window.location.origin)
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== '') {
        url.searchParams.append(key, String(value))
      }
    })
  }
  return url.toString()
}

/**
 * 构建 API 完整 URL 前缀（用于 fetch 直连场景，如 SSE 流式对话）
 */
export function buildApiUrl(path: string): string {
  return API_BASE + path
}

/**
 * 统一请求函数
 * 自动解包 { code, message, data } 信封：code !== 0 时抛出 ApiError
 * @param path API 路径
 * @param options 请求选项
 * @returns 解包后的 data 数据
 */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { params, ...fetchOptions } = options
  const url = buildUrl(path, params)

  // 默认请求头(自动合并桌面端 token,web 模式无侵入)
  const headers: Record<string, string> = {
    'Accept': 'application/json',
    'Content-Type': 'application/json',
    ...authHeaders(),
    ...((fetchOptions.headers as Record<string, string>) || {}),
  }

  try {
    const response = await fetch(url, {
      ...fetchOptions,
      headers,
    })

    // 处理 HTTP 错误
    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}))
      throw new ApiError(
        errorData.message || `HTTP ${response.status}: ${response.statusText}`,
        response.status,
        errorData
      )
    }

    // 204 No Content
    if (response.status === 204) {
      return undefined as T
    }

    const body = await response.json()

    // 解包 { code, message, data } 信封
    if (body && typeof body === 'object' && typeof body.code === 'number' && 'data' in body) {
      if (body.code !== 0) {
        throw new ApiError(body.message || '请求失败', body.code, body)
      }
      return body.data as T
    }

    return body as T
  } catch (error) {
    if (error instanceof ApiError) {
      throw error
    }
    // 网络错误
    throw new ApiError(
      error instanceof Error ? error.message : '网络请求失败，请检查网络连接',
      0
    )
  }
}

/**
 * 自定义 API 错误类
 */
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: Record<string, unknown>
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * GET 请求快捷方法
 */
export function get<T>(path: string, params?: Record<string, string | number | boolean | undefined>): Promise<T> {
  return request<T>(path, { method: 'GET', params })
}

/**
 * POST 请求快捷方法
 */
export function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

/**
 * PUT 请求快捷方法
 */
export function put<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

/**
 * DELETE 请求快捷方法
 */
export function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

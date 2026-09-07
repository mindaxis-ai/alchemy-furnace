import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from './api'
import {
  confirmFusion,
  consumePill,
  getOperation,
  getPillItem,
  previewFusion,
  resolveLegacyPill,
} from './pillInventoryService'

// 后端信封 { code, message, data }；jsdom 下 request() 把相对路径解析到 http://localhost
function okEnvelope(data: unknown) {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    headers: new Headers(),
    json: async () => ({ code: 0, message: 'ok', data }),
  }
}

function notFoundEnvelope() {
  return {
    ok: false,
    status: 404,
    statusText: 'Not Found',
    headers: new Headers(),
    json: async () => ({ code: 404, message: 'pill not found', data: null }),
  }
}

describe('resolveLegacyPill', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const LEGACY_ID = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
  const RECIPE_ID = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'

  it('queries the explicit legacy entry (GET /pills/:uuid) and returns the recipe pointer', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      okEnvelope({ entity_type: 'recipe', recipe_id: RECIPE_ID })
    )
    vi.stubGlobal('fetch', fetchMock)

    const pointer = await resolveLegacyPill(LEGACY_ID)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, options] = fetchMock.mock.calls[0]
    // jsdom 默认 origin 是 http://localhost:3000；断言路径与查询方式即可
    expect(url).toBe(`http://localhost:3000/api/v1/pills/${LEGACY_ID}`)
    expect(options.method).toBe('GET')
    expect(pointer).toEqual({ entity_type: 'recipe', recipe_id: RECIPE_ID })
  })

  it('propagates 404 unchanged (no mapping: caller decides to show not-found)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(notFoundEnvelope()))

    const error = await resolveLegacyPill(LEGACY_ID).catch((caught: unknown) => caught)

    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).status).toBe(404)
  })
})

describe('pill inventory UUID resource plumbing', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // 资源身份一律 UUID 字符串；禁止 Number()/parseInt() 数字化（011 契约回归守卫）
  const AGENT_ID = '11111111-1111-4111-8111-111111111111'
  const ITEM_ID = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
  const OPERATION_ID = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
  const PREVIEW_ID = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
  const KEY = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd'

  it('getPillItem / getOperation: item/operation UUID 原样进查询 URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(okEnvelope({ id: ITEM_ID }))
    vi.stubGlobal('fetch', fetchMock)

    await getPillItem(ITEM_ID)
    await getOperation(OPERATION_ID)

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[0][0]).toBe(`http://localhost:3000/api/v1/pill-items/${ITEM_ID}`)
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe('GET')
    expect(fetchMock.mock.calls[1][0]).toBe(`http://localhost:3000/api/v1/pill-operations/${OPERATION_ID}`)
  })

  it('consumePill: agent/item UUID 原样进 URL 与 body（幂等头为 UUID key）', async () => {
    const fetchMock = vi.fn().mockResolvedValue(okEnvelope({ operation_id: KEY }))
    vi.stubGlobal('fetch', fetchMock)

    await consumePill(KEY, AGENT_ID, ITEM_ID, { weight: 3, sortOrder: 1 })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toBe(`http://localhost:3000/api/v1/agents/${AGENT_ID}/consume`)
    const options = fetchMock.mock.calls[0][1] as RequestInit
    expect(options.method).toBe('POST')
    expect(JSON.parse(String(options.body))).toEqual({ item_id: ITEM_ID, weight: 3, sort_order: 1 })
    expect((options.headers as Record<string, string>)['Idempotency-Key']).toBe(KEY)
  })

  it('previewFusion / confirmFusion: item_ids 数组与 preview_id UUID 原样进 body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      okEnvelope({ preview_id: PREVIEW_ID, expires_at: '', name: '', description: '', skill_schema: {}, operator: { id: AGENT_ID, name: '' }, model: '', degraded: false })
    )
    vi.stubGlobal('fetch', fetchMock)

    await previewFusion([ITEM_ID])
    await confirmFusion(KEY, PREVIEW_ID, '融合金丹')

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[0][0]).toBe('http://localhost:3000/api/v1/fusion/previews')
    expect(JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))).toEqual({ item_ids: [ITEM_ID] })
    expect(fetchMock.mock.calls[1][0]).toBe('http://localhost:3000/api/v1/fusion/confirm')
    expect(JSON.parse(String((fetchMock.mock.calls[1][1] as RequestInit).body))).toEqual({
      preview_id: PREVIEW_ID,
      name: '融合金丹',
      description: '',
    })
  })
})

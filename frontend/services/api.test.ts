import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError, request } from './api'

describe('request', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('exposes the stable error code returned by the API', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: 409,
      error_code: 'service.chat.agent_inactive',
      message: 'agent is inactive',
      request_id: 'request-123',
      data: null,
    }), { status: 409, statusText: 'Conflict' })))

    const error = await request('/chat').catch((caught: unknown) => caught)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      status: 409,
      errorCode: 'service.chat.agent_inactive',
    })
  })

  it('leaves errorCode empty when the request does not receive a response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network unavailable')))

    const error = await request('/chat').catch((caught: unknown) => caught)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 0 })
    expect((error as ApiError).errorCode).toBeUndefined()
  })
})

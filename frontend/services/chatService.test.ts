import { afterEach, describe, expect, it, vi } from 'vitest'

import { streamChatMessage, type StreamHandlers } from '@/services/chatService'

function handlers(overrides: Partial<StreamHandlers> = {}): StreamHandlers {
  return {
    onChunk: vi.fn(),
    onDone: vi.fn(),
    onStopped: vi.fn(),
    onError: vi.fn(),
    onInterrupted: vi.fn(),
    ...overrides,
  }
}

function sseResponse(body: string): Response {
  return new Response(body, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  })
}

describe('chat SSE transport boundaries', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reports terminal transport interruption after a nonterminal member error and no turn_done', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      'event: speaker_start',
      'data: {"agent_id":"agent-a","agent_name":"Alpha","agent_avatar":"/alpha.png"}',
      '',
      'event: chunk',
      'data: {"agent_id":"agent-a","agent_name":"Alpha","agent_avatar":"/alpha.png","content":"partial"}',
      '',
      'event: error',
      'data: {"agent_id":"agent-a","agent_name":"Alpha","terminal":false,"content":"member failed"}',
      '',
      '',
    ].join('\n'))))
    const onError = vi.fn()
    const onInterrupted = vi.fn()

    await streamChatMessage('session', 'question', handlers({ onError, onInterrupted }))

    expect(onError).toHaveBeenCalledWith('member failed', expect.objectContaining({
      terminal: false,
      agent_id: 'agent-a',
    }))
    expect(onInterrupted).toHaveBeenCalledTimes(1)
  })

  it('delivers identity-bearing chunks instead of relying on a global current speaker', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      'event: chunk',
      'data: {"agent_id":"agent-b","agent_name":"Beta","agent_avatar":"/beta.png","content":"reply"}',
      '',
      'event: turn_done',
      'data: {"spoke":1}',
      '',
      '',
    ].join('\n'))))
    const onChunk = vi.fn()

    await streamChatMessage('session', 'question', handlers({ onChunk }))

    expect(onChunk).toHaveBeenCalledWith({
      agent_id: 'agent-b',
      agent_name: 'Beta',
      agent_avatar: '/beta.png',
      content: 'reply',
    })
  })

  it('serializes the explicit retry contract', async () => {
    const fetchMock = vi.fn().mockResolvedValue(sseResponse('event: done\ndata: {}\n\n'))
    vi.stubGlobal('fetch', fetchMock)

    await (streamChatMessage as unknown as (
      sessionId: string,
      content: string,
      handlers: StreamHandlers,
      options: { retry: boolean },
    ) => Promise<void>)('session', 'same question', handlers(), { retry: true })

    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect(JSON.parse(String(request.body))).toEqual({ content: 'same question', retry: true })
  })

  it('opts into prompt debugging only when requested', async () => {
    const fetchMock = vi.fn().mockResolvedValue(sseResponse('event: done\ndata: {}\n\n'))
    vi.stubGlobal('fetch', fetchMock)

    await (streamChatMessage as unknown as (
      sessionId: string,
      content: string,
      handlers: StreamHandlers,
      options: { debugPrompt: boolean },
    ) => Promise<void>)('session', 'inspect me', handlers(), { debugPrompt: true })

    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect(JSON.parse(String(request.body))).toEqual({ content: 'inspect me', debug_prompt: true })
  })

  it('delivers prompt_debug before the answer chunks', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      'event: prompt_debug',
      'data: {"agent_id":"agent-a","agent_name":"Alpha","model":"test-model","messages":[{"role":"system","content":"STRICT_DEBUG_SYSTEM_PROMPT"}],"generation":{"max_tokens":128,"max_sentences":2}}',
      '',
      'event: chunk',
      'data: {"content":"answer"}',
      '',
      'event: done',
      'data: {}',
      '',
      '',
    ].join('\n'))))
    const onPromptDebug = vi.fn()
    const onChunk = vi.fn()

    await streamChatMessage('session', 'question', handlers({
      onChunk,
      ...({ onPromptDebug } as Record<string, unknown>),
    } as Partial<StreamHandlers>))

    expect(onPromptDebug).toHaveBeenCalledWith(expect.objectContaining({
      agent_name: 'Alpha',
      model: 'test-model',
    }))
    expect(onPromptDebug.mock.invocationCallOrder[0]).toBeLessThan(onChunk.mock.invocationCallOrder[0])
  })

  it('acknowledges persisted user state before later stream events', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      'event: accepted',
      'data: {}',
      '',
      'event: done',
      'data: {}',
      '',
      '',
    ].join('\n'))))
    const onAccepted = vi.fn()
    const onDone = vi.fn()

    await streamChatMessage('session', 'question', handlers({ onAccepted, onDone }))

    expect(onAccepted).toHaveBeenCalledTimes(1)
    expect(onAccepted.mock.invocationCallOrder[0]).toBeLessThan(onDone.mock.invocationCallOrder[0])
  })

  it('defaults an error without an explicit recovery mode to no recovery', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse([
      'event: error',
      'data: {"terminal":true,"content":"safe failure"}',
      '',
      '',
    ].join('\n'))))
    const onError = vi.fn()

    await streamChatMessage('session', 'question', handlers({ onError }))

    expect(onError).toHaveBeenCalledWith('safe failure', expect.objectContaining({
      terminal: true,
      recovery: 'none',
    }))
  })
})

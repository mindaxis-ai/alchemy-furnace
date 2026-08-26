import { describe, expect, it } from 'vitest'
import {
  chatLobbyHref,
  chatSessionHref,
  parseChatSessionId,
  parseLegacyChatPath,
} from '@/lib/chat-route'

const id = '11111111-1111-4111-8111-111111111111'

describe('chat route contract', () => {
  it('builds the only canonical lobby and session URLs', () => {
    expect(chatLobbyHref()).toBe('/chat')
    expect(chatSessionHref(id)).toBe(`/chat?session=${id}`)
  })

  it('parses a valid session and rejects missing, placeholder, malformed values', () => {
    expect(parseChatSessionId(new URLSearchParams(`session=${id}`))).toBe(id)
    expect(parseChatSessionId(new URLSearchParams())).toBeUndefined()
    expect(parseChatSessionId(new URLSearchParams('session=_'))).toBeUndefined()
    expect(parseChatSessionId(new URLSearchParams('session=not-a-uuid'))).toBeUndefined()
  })

  it('recognizes only a valid historical chat path', () => {
    expect(parseLegacyChatPath(`/chat/${id}`)).toBe(id)
    expect(parseLegacyChatPath('/chat')).toBeUndefined()
    expect(parseLegacyChatPath('/agents/11111111-1111-4111-8111-111111111111')).toBeUndefined()
    expect(parseLegacyChatPath('/chat/_')).toBeUndefined()
  })
})

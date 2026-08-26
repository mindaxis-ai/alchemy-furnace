import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { HashRedirect } from '@/components/hash-redirect'

const replace = vi.hoisted(() => vi.fn())

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace }),
}))

function renderWithHash(hash: string) {
  window.location.hash = hash
  render(<HashRedirect />)
}

describe('HashRedirect legacy chat links', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.location.hash = ''
  })

  afterEach(() => {
    window.location.hash = ''
  })

  it('normalizes a legacy #/chat/<uuid> to the canonical query URL', () => {
    renderWithHash('#/chat/11111111-1111-4111-8111-111111111111')
    expect(replace).toHaveBeenCalledOnce()
    expect(replace).toHaveBeenCalledWith(
      '/chat?session=11111111-1111-4111-8111-111111111111',
    )
  })

  it('sends the static-export placeholder #/chat/_ back to the lobby', () => {
    renderWithHash('#/chat/_')
    expect(replace).toHaveBeenCalledOnce()
    expect(replace).toHaveBeenCalledWith('/chat')
  })

  it('sends a malformed #/chat/<id> back to the lobby without throwing', () => {
    renderWithHash('#/chat/not-a-uuid')
    expect(replace).toHaveBeenCalledOnce()
    expect(replace).toHaveBeenCalledWith('/chat')
  })
})

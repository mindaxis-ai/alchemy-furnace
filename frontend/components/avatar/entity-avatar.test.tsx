import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { EntityAvatar } from '@/components/avatar/entity-avatar'

describe('EntityAvatar', () => {
  afterEach(() => cleanup())

  it('renders image for a valid source', () => {
    render(<EntityAvatar name="太上老君" src="https://example.com/a.png" size="md" />)
    expect(screen.getByRole('img', { name: '太上老君' })).toHaveAttribute('src', 'https://example.com/a.png')
  })

  it('suppresses the referrer for CDN avatar URLs with transformation suffixes', () => {
    const src = 'https://pics2.baidu.com/feed/avatar.jpeg@f_auto?token=example'
    render(<EntityAvatar name="太上老君" src={src} size="md" />)
    const image = screen.getByRole('img', { name: '太上老君' })
    expect(image).toHaveAttribute('src', src)
    expect(image).toHaveAttribute('referrerpolicy', 'no-referrer')
  })

  it('falls back to initial once image errors', () => {
    render(<EntityAvatar name="太上老君" src="https://example.com/a.png" size="md" />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('太')).toBeInTheDocument()
  })

  it('does not render an img for an invalid source', () => {
    render(<EntityAvatar name="孙悟空" src="javascript:bad" size="sm" />)
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('孙')).toBeInTheDocument()
  })

  it('does not render an img once src changes from valid to invalid', () => {
    const { rerender } = render(<EntityAvatar name="孙悟空" src="https://example.com/a.png" size="sm" />)
    expect(screen.getByRole('img')).toBeInTheDocument()
    rerender(<EntityAvatar name="孙悟空" src="/relative.png" size="sm" />)
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('孙')).toBeInTheDocument()
  })

  it('resets broken state and restores the image when src changes to a valid URL', () => {
    const { rerender } = render(<EntityAvatar name="太上老君" src="https://example.com/a.png" size="md" />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.queryByRole('img')).toBeNull()
    rerender(<EntityAvatar name="太上老君" src="https://example.com/b.png" size="md" />)
    expect(screen.getByRole('img', { name: '太上老君' })).toHaveAttribute('src', 'https://example.com/b.png')
  })

  it('renders the bot icon as fallback when requested', () => {
    render(<EntityAvatar name="道人" src="javascript:bad" size="md" fallback="bot" />)
    expect(screen.queryByRole('img')).toBeNull()
    expect(document.querySelector('svg.lucide-bot')).not.toBeNull()
  })

  it('uses the provided alt for the image', () => {
    render(<EntityAvatar name="太上老君" src="https://example.com/a.png" size="md" alt="自定义头像" />)
    expect(screen.getByRole('img', { name: '自定义头像' })).toBeInTheDocument()
  })

  it('renders initial fallback for empty src', () => {
    render(<EntityAvatar name="太上老君" src="" size="md" />)
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('太')).toBeInTheDocument()
  })
})

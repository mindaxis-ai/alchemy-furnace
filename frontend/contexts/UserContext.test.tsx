import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UserProvider, useUser } from '@/contexts/UserContext'
import type { UserProfile } from '@/services/userService'

const td = vi.hoisted(() => ({
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))

vi.mock('@/services/userService', () => ({
  getProfile: td.getProfile,
  updateProfile: td.updateProfile,
}))

const sampleProfile: UserProfile = {
  display_name: 'Yao',
  bio: 'Alchemist',
  avatar: '',
  updated_at: '2026-08-20T00:00:00Z',
}

const renderLog: string[] = []

function Probe() {
  const { profile, loading, error } = useUser()
  renderLog.push(`loading=${loading}`)
  return (
    <div>
      <span data-testid="loading">{String(loading)}</span>
      <span data-testid="profile">{profile ? profile.display_name : 'none'}</span>
      <span data-testid="error">{error ?? 'none'}</span>
    </div>
  )
}

function Actions() {
  const { fetchProfile, updateProfile } = useUser()
  return (
    <div>
      <button data-testid="refetch" onClick={() => void fetchProfile()} />
      <button
        data-testid="update"
        onClick={() => void updateProfile({ display_name: 'New Name' })}
      />
    </div>
  )
}

describe('UserContext', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    vi.resetAllMocks()
    renderLog.length = 0
    td.getProfile.mockResolvedValue(sampleProfile)
    td.updateProfile.mockResolvedValue({ ...sampleProfile, display_name: 'New Name' })
  })

  it('is already loading on the very first render (auto fetch on mount)', async () => {
    td.getProfile.mockReturnValue(new Promise<UserProfile>(() => {}))
    render(
      <UserProvider>
        <Probe />
      </UserProvider>,
    )

    expect(renderLog[0]).toBe('loading=true')
    expect(screen.getByTestId('loading')).toHaveTextContent('true')
    expect(screen.getByTestId('profile')).toHaveTextContent('none')
  })

  it('fills the profile and clears loading after a successful fetch', async () => {
    render(
      <UserProvider>
        <Probe />
      </UserProvider>,
    )

    expect(await screen.findByText('Yao')).toBeInTheDocument()
    expect(screen.getByTestId('loading')).toHaveTextContent('false')
    expect(screen.getByTestId('error')).toHaveTextContent('none')
  })

  it('exposes the error and stops loading when the fetch fails', async () => {
    td.getProfile.mockRejectedValue(new Error('boom'))
    render(
      <UserProvider>
        <Probe />
      </UserProvider>,
    )

    expect(await screen.findByText('boom')).toBeInTheDocument()
    expect(screen.getByTestId('loading')).toHaveTextContent('false')
    expect(screen.getByTestId('profile')).toHaveTextContent('none')
  })

  it('does not set state after unmount while the fetch is still pending', async () => {
    let resolveProfile!: (p: UserProfile) => void
    td.getProfile.mockReturnValue(
      new Promise<UserProfile>((res) => {
        resolveProfile = res
      }),
    )
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { unmount } = render(
      <UserProvider>
        <Probe />
      </UserProvider>,
    )
    unmount()
    await act(async () => {
      resolveProfile(sampleProfile)
    })
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('manual fetchProfile re-enters loading and updateProfile refreshes the profile', async () => {
    render(
      <UserProvider>
        <Probe />
        <Actions />
      </UserProvider>,
    )
    expect(await screen.findByText('Yao')).toBeInTheDocument()

    // 手动 refetch: 挂起期间 loading 恢复 true
    let resolveRefetch!: (p: UserProfile) => void
    td.getProfile.mockReturnValue(
      new Promise<UserProfile>((res) => {
        resolveRefetch = res
      }),
    )
    act(() => {
      screen.getByTestId('refetch').click()
    })
    expect(screen.getByTestId('loading')).toHaveTextContent('true')
    await act(async () => {
      resolveRefetch(sampleProfile)
    })
    expect(screen.getByTestId('loading')).toHaveTextContent('false')

    // updateProfile 成功后刷新本地 profile
    await act(async () => {
      screen.getByTestId('update').click()
    })
    expect(td.updateProfile).toHaveBeenCalledWith({ display_name: 'New Name' })
    expect(screen.getByTestId('profile')).toHaveTextContent('New Name')
  })
})

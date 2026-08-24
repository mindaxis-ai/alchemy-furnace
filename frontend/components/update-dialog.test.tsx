import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UpdateDialog } from '@/components/update-dialog'
import type { UpdateCheckResult } from '@/services/systemService'

const td = vi.hoisted(() => ({
  checkUpdate: vi.fn(),
  applyUpdate: vi.fn(),
  getUpdateProgress: vi.fn(),
}))

vi.mock('@/services/systemService', () => ({
  checkUpdate: td.checkUpdate,
  applyUpdate: td.applyUpdate,
  getUpdateProgress: td.getUpdateProgress,
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

const availableResult: UpdateCheckResult = {
  has_update: true,
  current_version: '1.0.0',
  latest_version: '1.2.3',
  asset_size: 2048,
  asset_name: 'AlchemyFurnace-mac-arm64.dmg',
  notes: '',
  page_url: 'https://example.com/releases',
}

describe('UpdateDialog', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    vi.resetAllMocks()
    td.checkUpdate.mockResolvedValue(availableResult)
    td.applyUpdate.mockResolvedValue({ message: 'ok' })
    td.getUpdateProgress.mockResolvedValue({ progress: 0 })
  })

  it('auto-checks on mount and shows the available version', async () => {
    render(<UpdateDialog onClose={() => {}} />)

    expect(td.checkUpdate).toHaveBeenCalledTimes(1)
    expect(await screen.findByText(/1\.2\.3/)).toBeInTheDocument()
    expect(screen.getByText(/1\.0\.0/)).toBeInTheDocument()
  })

  it('shows the latest state when there is no update', async () => {
    td.checkUpdate.mockResolvedValue({ ...availableResult, has_update: false })
    render(<UpdateDialog onClose={() => {}} />)

    expect(await screen.findByText('latest')).toBeInTheDocument()
  })

  it('shows the disabled state for dev builds', async () => {
    td.checkUpdate.mockResolvedValue({ ...availableResult, notes: '开发构建未启用更新' })
    render(<UpdateDialog onClose={() => {}} />)

    expect(await screen.findByText('disabled')).toBeInTheDocument()
  })

  it('shows the failure state with the error message when the check rejects', async () => {
    td.checkUpdate.mockRejectedValue(new Error('network down'))
    render(<UpdateDialog onClose={() => {}} />)

    expect(await screen.findByText('failed')).toBeInTheDocument()
    expect(screen.getByText('network down')).toBeInTheDocument()
  })

  it('does not set state after unmount while the check is pending', async () => {
    let resolveCheck!: (r: UpdateCheckResult) => void
    td.checkUpdate.mockReturnValue(
      new Promise<UpdateCheckResult>((res) => {
        resolveCheck = res
      }),
    )
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { unmount } = render(<UpdateDialog onClose={() => {}} />)
    unmount()
    await act(async () => {
      resolveCheck(availableResult)
    })
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })
})

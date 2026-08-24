import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import FusionPage from '@/app/(main)/fusion/page'
import type { SystemConfig } from '@/services/systemService'

const td = vi.hoisted(() => ({
  push: vi.fn(),
  listPills: vi.fn(),
  createPill: vi.fn(),
  deletePill: vi.fn(),
  fusePills: vi.fn(),
  withLineage: vi.fn(),
  listProviders: vi.fn(),
  listModels: vi.fn(),
  updateModel: vi.fn(),
  getSystemConfig: vi.fn(),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: td.push }),
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

vi.mock('@/services/pillService', () => ({
  listPills: td.listPills,
  createPill: td.createPill,
  deletePill: td.deletePill,
}))

vi.mock('@/services/fusionService', () => ({
  fusePills: td.fusePills,
  withLineage: td.withLineage,
}))

vi.mock('@/services/modelService', () => ({
  listProviders: td.listProviders,
  listModels: td.listModels,
  updateModel: td.updateModel,
}))

vi.mock('@/services/systemService', () => ({
  getSystemConfig: td.getSystemConfig,
}))

// 炉火动画内部是 canvas,jsdom 跑不了;替换为桩
vi.mock('@/components/alchemy/bagua-furnace', () => ({
  BaguaFurnace: () => <div data-testid="furnace" />,
}))

vi.mock('@/components/fusion/fusion-preview-modal', () => ({
  FusionPreviewModal: () => <div data-testid="preview-modal" />,
}))

const configuredConfig: SystemConfig = {
  version: '1.0.0',
  models: [],
  default_model: 'm',
  synthesis_model: 'm',
  fusion_model: 'm',
  fusion_model_info: {
    configured: true,
    model_name: 'gpt-fusion',
    model_display_name: 'FusionModel X',
    provider_name: 'openai',
    provider_display_name: 'OpenAI',
  },
}

describe('FusionPage', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    vi.resetAllMocks()
    td.listPills.mockResolvedValue({ list: [], total: 0 })
    td.listProviders.mockResolvedValue({ list: [{ id: 'p1' }], total: 1 })
    td.getSystemConfig.mockResolvedValue(configuredConfig)
  })

  it('shows the configured fusion model after the initial load', async () => {
    render(<FusionPage />)

    expect(await screen.findByText(/FusionModel X/)).toBeInTheDocument()
    expect(screen.getByText(/OpenAI/)).toBeInTheDocument()
  })

  it('shows the not-configured warning when the config request fails', async () => {
    td.getSystemConfig.mockRejectedValue(new Error('config unavailable'))
    render(<FusionPage />)

    expect(await screen.findByText('preview.noFusionModel')).toBeInTheDocument()
  })

  it('does not set state after unmount while initial loads are pending', async () => {
    let resolveConfig!: (c: SystemConfig) => void
    td.getSystemConfig.mockReturnValue(
      new Promise<SystemConfig>((res) => {
        resolveConfig = res
      }),
    )
    td.listPills.mockReturnValue(new Promise(() => {}))
    td.listProviders.mockReturnValue(new Promise(() => {}))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { unmount } = render(<FusionPage />)
    unmount()
    await act(async () => {
      resolveConfig(configuredConfig)
    })
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })
})

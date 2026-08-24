import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { FireEffectPanel } from '@/components/settings/fire-effect-panel'
import {
  FIRE_EFFECT_STORAGE_KEY,
  SMOKE_LEVEL_STORAGE_KEY,
} from '@/lib/fire-effect-pref'

const previewProps = vi.hoisted(() => ({
  calls: [] as Array<{ effectId: string; smokeLevel: number }>,
}))

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}))

// 预览组件内部是 canvas 动画,jsdom 跑不了;替换为记录 props 的桩
vi.mock('@/components/alchemy/fire-effect-preview', () => ({
  FireEffectPreview: (props: { effectId: string; smokeLevel: number }) => {
    previewProps.calls.push({ effectId: props.effectId, smokeLevel: props.smokeLevel })
    return <div data-testid="preview" />
  },
}))

describe('FireEffectPanel', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    previewProps.calls.length = 0
    window.localStorage.clear()
  })

  it('reflects the stored fire effect and smoke level on mount', () => {
    window.localStorage.setItem(FIRE_EFFECT_STORAGE_KEY, 'ember')
    window.localStorage.setItem(SMOKE_LEVEL_STORAGE_KEY, '0.2')
    render(<FireEffectPanel />)

    expect(screen.getByRole('button', { name: /options\.ember\.label/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('slider')).toHaveValue('20')
    expect(previewProps.calls.at(-1)).toEqual({ effectId: 'ember', smokeLevel: 0.2 })
  })

  it('falls back to defaults when nothing is stored', () => {
    render(<FireEffectPanel />)

    expect(screen.getByRole('button', { name: /options\.plume\.label/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('slider')).toHaveValue('55')
  })

  it('persists a picked effect and updates the highlight immediately', () => {
    render(<FireEffectPanel />)

    fireEvent.click(screen.getByRole('button', { name: /options\.veil\.label/ }))

    expect(window.localStorage.getItem(FIRE_EFFECT_STORAGE_KEY)).toBe('veil')
    expect(screen.getByRole('button', { name: /options\.veil\.label/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(previewProps.calls.at(-1)).toEqual({ effectId: 'veil', smokeLevel: 0.55 })
  })

  it('persists a picked smoke level and updates the slider', () => {
    render(<FireEffectPanel />)

    fireEvent.change(screen.getByRole('slider'), { target: { value: '80' } })

    expect(window.localStorage.getItem(SMOKE_LEVEL_STORAGE_KEY)).toBe('0.8')
    expect(screen.getByRole('slider')).toHaveValue('80')
    expect(previewProps.calls.at(-1)).toEqual({ effectId: 'plume', smokeLevel: 0.8 })
  })
})

import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BaguaFurnace } from '@/components/alchemy/bagua-furnace'
import {
  FIRE_EFFECT_STORAGE_KEY,
  SMOKE_LEVEL_STORAGE_KEY,
} from '@/lib/fire-effect-pref'

const captured = vi.hoisted(() => ({
  fire: [] as Array<{ effectId: string; pixelRatio: number }>,
  smoke: [] as Array<{ level: number; pixelRatio: number }>,
}))

// canvas 子组件在 jsdom 跑不了动画;替换为记录 props 的桩
vi.mock('@/components/alchemy/bagua-furnace-fire', () => ({
  BaguaFurnaceFire: (props: { effectId: string; pixelRatio: number }) => {
    captured.fire.push({ effectId: props.effectId, pixelRatio: props.pixelRatio })
    return <div data-testid="fire" />
  },
}))

vi.mock('@/components/alchemy/bagua-furnace-smoke', () => ({
  BaguaFurnaceSmoke: (props: { level: number; pixelRatio: number }) => {
    captured.smoke.push({ level: props.level, pixelRatio: props.pixelRatio })
    return <div data-testid="smoke" />
  },
}))

vi.mock('next/image', () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    return <span data-alt={String(props.alt ?? '')} style={props.style as React.CSSProperties} />
  },
}))

const windows = [
  { id: 'center', x: 50, width: 10, top: 50, height: 10, phase: 0 },
]

function stubMatchMedia() {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia
}

describe('BaguaFurnace', () => {
  afterEach(() => cleanup())

  beforeEach(() => {
    captured.fire.length = 0
    captured.smoke.length = 0
    window.localStorage.clear()
    stubMatchMedia()
  })

  it('passes the stored fire effect and smoke level to the canvas layers', () => {
    window.localStorage.setItem(FIRE_EFFECT_STORAGE_KEY, 'ember')
    window.localStorage.setItem(SMOKE_LEVEL_STORAGE_KEY, '0.3')
    render(<BaguaFurnace alt="furnace" windows={windows} />)

    expect(captured.fire.at(-1)?.effectId).toBe('ember')
    expect(captured.smoke.at(-1)?.level).toBe(0.3)
  })

  it('falls back to defaults when nothing is stored', () => {
    render(<BaguaFurnace alt="furnace" windows={windows} />)

    expect(captured.fire.at(-1)?.effectId).toBe('plume')
    expect(captured.smoke.at(-1)?.level).toBe(0.55)
  })

  it('follows same-tab preference changes without a remount', () => {
    render(<BaguaFurnace alt="furnace" windows={windows} />)

    // 模拟设置页写入: localStorage + CustomEvent（与 fire-effect-pref.setFireEffect 同序）
    act(() => {
      window.localStorage.setItem(FIRE_EFFECT_STORAGE_KEY, 'veil')
      window.dispatchEvent(new CustomEvent('alchemy:fire-effect-change', { detail: 'veil' }))
    })

    expect(captured.fire.at(-1)?.effectId).toBe('veil')
  })
})

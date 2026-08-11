/**
 * 炉火/烟雾偏好（006-fire-settings）
 *
 * localStorage 持久化用户在设置页选定的火效与烟雾浓度。SSR 安全（用
 * typeof window 守卫），非法/缺失值回退默认。多标签页同步：
 *  - storage event：跨标签页生效
 *  - CustomEvent：同标签页立即生效（localStorage 不会触发 storage 给写者）
 */

import type { FireEffectId } from '@/components/alchemy/fire-effects'

export type { FireEffectId }

export const FIRE_EFFECT_STORAGE_KEY = 'alchemy.fireEffect'
export const SMOKE_LEVEL_STORAGE_KEY = 'alchemy.smokeLevel'

export const FIRE_EFFECT_OPTIONS: readonly FireEffectId[] = [
  'plume',
  'ember',
  'flicker',
  'veil',
  'classic',
  'sanmei',
  'spark',
  'dragon',
] as const

export const DEFAULT_FIRE_EFFECT: FireEffectId = 'plume'
export const DEFAULT_SMOKE_LEVEL = 0.55

/** 同标签页内的瞬时同步事件（CustomEvent） */
export const FIRE_EFFECT_CHANGE_EVENT = 'alchemy:fire-effect-change'
export const SMOKE_LEVEL_CHANGE_EVENT = 'alchemy:smoke-level-change'

function isValidEffect(v: unknown): v is FireEffectId {
  return typeof v === 'string' && (FIRE_EFFECT_OPTIONS as readonly string[]).includes(v)
}

export function getFireEffect(): FireEffectId {
  if (typeof window === 'undefined') return DEFAULT_FIRE_EFFECT
  try {
    const v = window.localStorage.getItem(FIRE_EFFECT_STORAGE_KEY)
    return isValidEffect(v) ? v : DEFAULT_FIRE_EFFECT
  } catch {
    return DEFAULT_FIRE_EFFECT
  }
}

export function setFireEffect(id: FireEffectId): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(FIRE_EFFECT_STORAGE_KEY, id)
    window.dispatchEvent(
      new CustomEvent<FireEffectId>(FIRE_EFFECT_CHANGE_EVENT, { detail: id })
    )
  } catch {
    // localStorage 不可用（隐私模式等）静默忽略
  }
}

export function getSmokeLevel(): number {
  if (typeof window === 'undefined') return DEFAULT_SMOKE_LEVEL
  try {
    const v = window.localStorage.getItem(SMOKE_LEVEL_STORAGE_KEY)
    if (v == null) return DEFAULT_SMOKE_LEVEL
    const n = Number(v)
    if (!Number.isFinite(n)) return DEFAULT_SMOKE_LEVEL
    return Math.min(1, Math.max(0, n))
  } catch {
    return DEFAULT_SMOKE_LEVEL
  }
}

export function setSmokeLevel(level: number): void {
  if (typeof window === 'undefined') return
  const clamped = Math.min(1, Math.max(0, level))
  try {
    window.localStorage.setItem(SMOKE_LEVEL_STORAGE_KEY, String(clamped))
    window.dispatchEvent(
      new CustomEvent<number>(SMOKE_LEVEL_CHANGE_EVENT, { detail: clamped })
    )
  } catch {
    // 静默
  }
}

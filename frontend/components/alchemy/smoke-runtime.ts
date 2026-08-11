/**
 * 烟雾运行时（006-fire-runtime）
 *
 * 同 fire-runtime 思路：把 smoke.tsx 里"调度器共享"的部分抽成纯函数，
 * 让预览能 1:1 复用主场景的烟逻辑。
 */

import { advanceIgnition, smoothstep } from './fire-runtime'
import type { FurnaceWindow } from './bagua-furnace-fire'

export interface Wisp {
  x: number
  y: number
  size: number
  life: number
  maxLife: number
  seed: number
  drift: number
  rise: number
}

/** 喷发 1 个 wisp（与 smoke.tsx 内部一致） */
export function spawnWisps(
  wisps: Wisp[],
  count: number,
  windows: FurnaceWindow[],
  size: { w: number; h: number }
): void {
  const { w, h } = size
  for (let i = 0; i < count; i++) {
    const win = windows[Math.floor(Math.random() * windows.length)]
    const wx = (win.x / 100) * w
    const wy = (win.top / 100) * h
    const ww = (win.width / 100) * w
    const sz = ww * (0.30 + Math.random() * 0.45)
    const life = 3.5 + Math.random() * 1.5
    wisps.push({
      x: wx + (Math.random() - 0.5) * ww * 0.9,
      y: wy + sz * 0.35 - Math.random() * 14,
      size: sz,
      life,
      maxLife: life,
      seed: Math.random() * Math.PI * 2,
      drift: (Math.random() - 0.5) * 70,
      rise: h * (0.065 + Math.random() * 0.03),
    })
  }
}

/** 推进所有 wisp（生命/位置/尺寸） */
export function updateWisps(wisps: Wisp[], dt: number): void {
  for (let i = wisps.length - 1; i >= 0; i--) {
    const w = wisps[i]!
    w.life -= dt
    if (w.life <= 0) {
      wisps.splice(i, 1)
      continue
    }
    const age = 1 - w.life / w.maxLife
    w.y -= w.rise * (0.7 + age * 0.9) * dt
    w.x +=
      (Math.sin(age * 2.2 + w.seed) * 18 +
        Math.sin(age * 5 + w.seed * 2) * 9 +
        w.drift * 0.22) *
      dt
    w.size += dt * 5
  }
}

/** 画所有 wisp（青灰色径向渐变）— alpha 与 level^0.5 关联，让低浓度看起来真的淡 */
export function drawWisps(
  ctx: CanvasRenderingContext2D,
  wisps: Wisp[],
  level: number
): void {
  const baseAlpha = 0.45 * Math.sqrt(Math.max(0, level))
  for (const w of wisps) {
    const age = 1 - w.life / w.maxLife
    const alpha =
      baseAlpha *
      smoothstep(0, 0.12, age) *
      (1 - smoothstep(0.55, 1, age))
    if (alpha < 0.02) continue
    const g = ctx.createRadialGradient(w.x, w.y, 0, w.x, w.y, w.size)
    g.addColorStop(0, `rgba(150, 156, 160, ${alpha})`)
    g.addColorStop(0.45, `rgba(128, 134, 138, ${alpha * 0.55})`)
    g.addColorStop(1, 'rgba(105, 110, 114, 0)')
    ctx.fillStyle = g
    ctx.beginPath()
    ctx.arc(w.x, w.y, w.size, 0, Math.PI * 2)
    ctx.fill()
  }
}

/** 烟的 spawn 控制（喷发率 + 上限）—— 主调度器和预览共用
 * 喷发率 = 40 * level^1.5,让低浓度真正稀薄(0.3 时仅 6.6/s) */
export function shouldSpawn(
  prog: number,
  level: number,
  wispsCount: number,
  effectiveBudget: number,
  dt: number
): number {
  if (level <= 0.001) return 0
  const spawnRate =
    40 * Math.pow(level, 1.5) * smoothstep(0.75, 1.0, prog)
  const target = spawnRate * dt
  const count = Math.floor(target) + (Math.random() < target % 1 ? 1 : 0)
  if (count <= 0 || wispsCount >= effectiveBudget) return 0
  return Math.min(count, effectiveBudget - wispsCount)
}

/** level 对应的最大同时活烟数（同 40 * level^1.5 曲线） */
export function smokeBudget(level: number): number {
  return Math.max(0, Math.floor(40 * Math.pow(level, 1.5)))
}

export { advanceIgnition } from './fire-runtime'

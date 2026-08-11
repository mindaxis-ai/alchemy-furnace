/**
 * 炉火运行时（006-fire-runtime）
 *
 * 把调度器里"所有 effect 共享"的渲染代码抽成纯函数：
 *  - 窗口裁剪（arch）
 *  - 暗腔 cavity 铺底
 *  - 床层呼吸 + 暖光 glow
 *  - 阶段增益计算（staged ignition）
 *
 * 主调度器（bagua-furnace-fire.tsx）和预览（fire-effect-preview.tsx）共用，
 * 保证 1:1 视觉一致。
 *
 * effect 自身只负责粒子（spawn/update/draw），不重复画 cavity/bed/glow。
 */

import type { FurnaceWindow } from './bagua-furnace-fire'
import type { StageGains } from './fire-effects'

/** 本地 smoothstep（避免循环 import fire-effects 调调度器） */
export function smoothstep(a: number, b: number, x: number): number {
  const t = Math.min(1, Math.max(0, (x - a) / (b - a)))
  return t * t * (3 - 2 * t)
}

/** 本地时间常数 + advanceIgnition 助手（避免 smoke-runtime 循环 import 调度器） */
const IGNITE_S = 0.6
const EXTINGUISH_S = 0.7
export function advanceIgnition(prog: number, target: number, dt: number): number {
  const dir = target > prog ? 1 : -1
  const dur = dir > 0 ? IGNITE_S : EXTINGUISH_S
  return Math.min(1, Math.max(0, prog + (dir * dt) / dur))
}

/** 给定窗口 + canvas 尺寸，算 rect */
export function windowRect(
  win: FurnaceWindow,
  size: { w: number; h: number }
): { wx: number; wy: number; ww: number; wh: number } {
  return {
    wx: (win.x / 100) * size.w,
    wy: (win.top / 100) * size.h,
    ww: (win.width / 100) * size.w,
    wh: (win.height / 100) * size.h,
  }
}

/** 推进 staged ignition + 算 4 个 layer gain */
export function stageGains(prog: number): StageGains {
  return {
    warm: smoothstep(0.0, 0.35, prog),
    bed: smoothstep(0.15, 0.55, prog),
    glow: smoothstep(0.35, 0.8, prog),
    ember: smoothstep(0.5, 0.85, prog),
  }
}

/** arch 窗口裁剪（与 ding.png cut-out mask 同款形状） */
export function archPath(
  ctx: CanvasRenderingContext2D,
  wx: number,
  wy: number,
  ww: number,
  wh: number
): void {
  const r = ww / 2
  ctx.moveTo(wx - r, wy + r)
  ctx.arc(wx, wy + r, r, Math.PI, 0)
  ctx.lineTo(wx + r, wy + wh)
  ctx.lineTo(wx - r, wy + wh)
  ctx.closePath()
}

/** 暗腔：每个窗口铺一层深色径向渐变，永远画 */
export function drawCavity(
  ctx: CanvasRenderingContext2D,
  windows: FurnaceWindow[],
  size: { w: number; h: number }
): void {
  for (const win of windows) {
    const { wx, wy, ww, wh } = windowRect(win, size)
    const cavity = ctx.createRadialGradient(
      wx,
      wy + wh * 0.55,
      0,
      wx,
      wy + wh * 0.55,
      Math.max(ww, wh) * 0.95
    )
    cavity.addColorStop(0, 'rgba(25, 8, 3, 0.88)')
    cavity.addColorStop(0.55, 'rgba(45, 16, 6, 0.7)')
    cavity.addColorStop(0.88, 'rgba(70, 28, 10, 0.4)')
    cavity.addColorStop(1, 'rgba(90, 35, 12, 0)')
    ctx.fillStyle = cavity
    ctx.fillRect(wx - ww / 2, wy, ww, wh)
  }
}

/** 床层呼吸 + 暖光：按 stageGains 调用 */
export function drawBedAndGlow(
  ctx: CanvasRenderingContext2D,
  windows: FurnaceWindow[],
  size: { w: number; h: number },
  gains: StageGains,
  time: number
): void {
  if (gains.bed > 0.01) {
    for (const win of windows) {
      const { wx, wy, ww, wh } = windowRect(win, size)
      const pulse =
        0.88 +
        0.12 *
          Math.sin(time * 7 + win.phase * 6.3) *
          Math.sin(time * 2.3 + win.phase * 12.6)
      const p = pulse * gains.bed
      const bed = ctx.createRadialGradient(
        wx,
        wy + wh * 0.94,
        0,
        wx,
        wy + wh * 0.94,
        ww * 0.68
      )
      bed.addColorStop(0, `rgba(255, 175, 60, ${0.45 * p})`)
      bed.addColorStop(0.28, `rgba(230, 90, 18, ${0.52 * p})`)
      bed.addColorStop(0.65, `rgba(180, 45, 8, ${0.28 * p})`)
      bed.addColorStop(1, 'rgba(120, 25, 5, 0)')
      ctx.fillStyle = bed
      ctx.beginPath()
      ctx.ellipse(wx, wy + wh * 0.94, ww * 0.5, wh * 0.18, 0, 0, Math.PI * 2)
      ctx.fill()
    }
  }
  if (gains.glow > 0.01) {
    for (const win of windows) {
      const { wx, wy, ww, wh } = windowRect(win, size)
      const flicker =
        (0.92 + 0.08 * Math.sin(time * 11 + win.phase * 9)) * gains.glow
      const g = ctx.createRadialGradient(
        wx,
        wy + wh * 0.55,
        0,
        wx,
        wy + wh * 0.55,
        Math.max(ww, wh) * 0.78
      )
      g.addColorStop(0, `rgba(255, 150, 40, ${0.55 * flicker})`)
      g.addColorStop(0.45, `rgba(240, 100, 20, ${0.38 * flicker})`)
      g.addColorStop(0.8, `rgba(180, 50, 10, ${0.16 * flicker})`)
      g.addColorStop(1, 'rgba(120, 30, 5, 0)')
      ctx.fillStyle = g
      ctx.fillRect(wx - ww / 2, wy, ww, wh)
    }
  }
}

/**
 * Dragon 龙形火
 *
 * 御火：渐变高尾火苗 + 重拖尾，丝滑优雅。
 *
 * 不同于 plume（短而尖）：
 *  - 长度比 plume 长 2x（length 2.5-3.5 vs plume 1.4-2.2）→ 丝带般延伸
 *  - vy 慢（0.8-1.5 vs plume 1.6-2.6）→ 缓慢飘升
 *  - 寿命长（1.2-1.8s）→ 拖尾持续可见
 *  - 颜色 ramp 走金/朱砂/暗红（无米白）
 *  - 粒子更宽（width 比 plume +20%）
 *  - 整体像丝绸般顺滑、有"龙蛇游动"的曲线感
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

const DRAGON_RAMP: Array<[number, [number, number, number]]> = [
  [0.0, [255, 230, 150]], // 浅金
  [0.3, [255, 170, 60]], // 金橙
  [0.6, [220, 80, 35]], // 朱砂
  [0.85, [150, 40, 15]], // 暗红
  [1.0, [80, 18, 8]], // 余烬
]

function ramp(age: number): [number, number, number] {
  for (let i = 1; i < DRAGON_RAMP.length; i++) {
    const stop = DRAGON_RAMP[i]!
    const prev = DRAGON_RAMP[i - 1]!
    if (age <= stop[0]) {
      const k = (age - prev[0]) / (stop[0] - prev[0])
      return [
        Math.round(prev[1][0] + (stop[1][0] - prev[1][0]) * k),
        Math.round(prev[1][1] + (stop[1][1] - prev[1][1]) * k),
        Math.round(prev[1][2] + (stop[1][2] - prev[1][2]) * k),
      ]
    }
  }
  return DRAGON_RAMP[DRAGON_RAMP.length - 1]![1]
}

const spawnAcc: number[] = []
let burstIn = 0

export const dragon: FireEffect = {
  id: 'dragon',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
    burstIn = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.ember < 0.01) return

    // 比 plume 慢喷发（每粒寿命长，靠累加出拖尾感）
    const baseRate = (budget.particles / 2.4) * gains.ember
    const winIdx = spawnAcc.length
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    spawnAcc[winIdx]! += baseRate * p.dt
    const steady = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= steady

    burstIn -= p.dt
    const bursting = burstIn <= 0
    if (bursting) burstIn = 1.4 + Math.random() * 0.8

    const total = steady + (bursting ? 2 + Math.floor(Math.random() * 3) : 0)
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 1.2 + Math.random() * 0.6
      const sizeBase = rect.ww * (0.05 + Math.random() * 0.04)
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.5,
        y: rect.wy + rect.wh * (0.82 + Math.random() * 0.16),
        vx: (Math.random() - 0.5) * rect.ww * 0.1,
        // 慢速上升
        vy: -rect.wh * (0.8 + Math.random() * 0.7) * (bursting ? 1.3 : 1),
        life,
        maxLife: life,
        size: sizeBase,
        seed: Math.random() * Math.PI * 2,
        // 长拖尾：length 2.5-3.5（plume 1.4-2.2）
        length: 2.5 + Math.random() * 1.0,
        // 横向摆动幅度大（"龙蛇游动"感）
        wob: rect.ww * (0.1 + Math.random() * 0.1),
        asym: (Math.random() - 0.5) * 0.3,
      })
    }
  },

  update(particles: Particle[], dt: number, time: number, _p: FireParams) {
    for (let i = particles.length - 1; i >= 0; i--) {
      const e = particles[i]!
      e.life -= dt
      if (e.life <= 0) {
        particles.splice(i, 1)
        continue
      }
      // 慢速横向摆动（更柔）
      e.x += (e.vx + Math.sin(time * 2.5 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      e.vy *= 1 - 0.25 * dt
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    ctx.globalCompositeOperation = 'source-over'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      // alpha 衰减更平缓（拖尾可见）
      const lifeAlpha = Math.pow(1 - age, 1.0)
      if (lifeAlpha < 0.02) continue

      // 粒子主体：宽度大（+20%）、高度用 length（2.5-3.5）
      const w = e.size * (1.0 + 0.2) * (1 - age * 0.4)
      const h = w * e.length
      const [r, g, b] = ramp(age)

      const cx = e.x + (e.asym ?? 0) * w * 0.4
      const cy = e.y - h * 0.5
      const grad = ctx.createLinearGradient(cx, cy + h * 0.5, cx, cy - h * 0.5)
      grad.addColorStop(0, `rgba(${r},${g},${b},${0.9 * lifeAlpha})`)
      grad.addColorStop(0.6, `rgba(${r},${g},${b},${0.45 * lifeAlpha})`)
      grad.addColorStop(1, `rgba(${r},${g},${b},0)`)
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.ellipse(cx, cy, w, h, 0, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

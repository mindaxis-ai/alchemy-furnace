/**
 * Classic 经典（"水感"旧版）
 *
 * 把 006 重构前的原版粒子系统原样保留：实色圆点 + 'lighter' 混合 +
 * 米白起点 EMBER_RAMP + 强横向 wobble + cavity wash 铺底。
 * 视觉上偏白偏水雾，是历史效果，给喜欢原版的人留个开关。
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

/** 原版 EMBER_RAMP：白热 → 琥珀 → 橙 → 橙红 → 深红 */
const EMBER_RAMP: Array<[number, [number, number, number]]> = [
  [0.0, [255, 246, 216]], // 白热
  [0.25, [255, 217, 122]], // 琥珀
  [0.55, [255, 154, 60]], // 橙
  [0.8, [224, 85, 26]], // 橙红
  [1.0, [140, 34, 8]], // 深红
]

function emberColor(age: number): [number, number, number] {
  for (let i = 1; i < EMBER_RAMP.length; i++) {
    const stop = EMBER_RAMP[i]!
    const prev = EMBER_RAMP[i - 1]!
    if (age <= stop[0]) {
      const k = (age - prev[0]) / (stop[0] - prev[0])
      return [
        Math.round(prev[1][0] + (stop[1][0] - prev[1][0]) * k),
        Math.round(prev[1][1] + (stop[1][1] - prev[1][1]) * k),
        Math.round(prev[1][2] + (stop[1][2] - prev[1][2]) * k),
      ]
    }
  }
  return EMBER_RAMP[EMBER_RAMP.length - 1]![1]
}

const spawnAcc: number[] = []
const burstIn: number[] = []

export const classic: FireEffect = {
  id: 'classic',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
    burstIn.length = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.ember < 0.01) return

    // 与原版同款喷发率: rate = particles / (0.85 * windows.length) * emberGain
    const rate = (budget.particles / (0.85 * 1)) * gains.ember
    const winIdx = spawnAcc.length
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    if (burstIn.length <= winIdx) burstIn.push(1 + Math.random() * 3)
    spawnAcc[winIdx]! += rate * p.dt
    const steady = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= steady

    burstIn[winIdx]! -= p.dt
    const bursting = burstIn[winIdx]! <= 0
    if (bursting) burstIn[winIdx]! = 2 + Math.random() * 2

    const total = steady + (bursting ? 8 + Math.floor(Math.random() * 9) : 0)
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 0.45 + Math.random() * 0.65
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.72,
        y: rect.wy + rect.wh * (0.82 + Math.random() * 0.16),
        vx: (Math.random() - 0.5) * rect.ww * 0.35,
        vy: -rect.wh * (1.3 + Math.random() * 1.4) * (bursting ? 1.5 : 1),
        life,
        maxLife: life,
        size: rect.ww * (0.015 + Math.random() * 0.03),
        seed: Math.random() * Math.PI * 2,
        wob: rect.ww * (0.3 + Math.random() * 0.4),
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
      e.x += (e.vx + Math.sin(time * 6 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      e.vy *= 1 - 0.12 * dt
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], p: FireParams) {
    // 原版 cavity 铺底（在 effect 内自管 warm wash，叠在调度器 cavity 之上）
    if (p.gains.warm > 0.01) {
      ctx.globalCompositeOperation = 'source-over'
      const { rect } = p
      const g = ctx.createRadialGradient(
        rect.wx,
        rect.wy + rect.wh * 0.6,
        0,
        rect.wx,
        rect.wy + rect.wh * 0.6,
        Math.max(rect.ww, rect.wh) * 0.7
      )
      g.addColorStop(0, `rgba(200, 80, 25, ${0.2 * p.gains.warm})`)
      g.addColorStop(0.6, `rgba(150, 45, 10, ${0.12 * p.gains.warm})`)
      g.addColorStop(1, 'rgba(100, 25, 5, 0)')
      ctx.fillStyle = g
      ctx.fillRect(rect.wx - rect.ww / 2, rect.wy, rect.ww, rect.wh)
    }

    // 粒子：原版 'lighter' 混合 + 米白起点 → 水雾感
    ctx.globalCompositeOperation = 'lighter'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      const [r, g, b] = emberColor(age)
      const alpha = Math.pow(1 - age, 1.1) * 0.9 * p.gains.ember
      const radius = Math.max(0.3, e.size * (1 - age * 0.9))
      ctx.fillStyle = `rgba(${r},${g},${b},${alpha})`
      ctx.beginPath()
      ctx.arc(e.x, e.y, radius, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

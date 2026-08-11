/**
 * Sanmei 三昧真火
 *
 * 炼丹奇幻：内焰冰蓝/雪青 + 中层朱砂 + 外焰冷青白。
 * 锥形 plume 同款形状,但调色板与 plume 完全不同(冷调主调,反差强烈)。
 *
 * 颜色 ramp（自内焰→外焰）：
 *  age=0.0  雪白蓝   (210, 240, 255)
 *  age=0.25 冰蓝     (90, 180, 240)
 *  age=0.55 朱砂     (210, 90, 60)
 *  age=0.8  砖红     (140, 50, 25)
 *  age=1.0  暗紫红   (60, 20, 25)
 *
 * 视觉上像"冷热共存"——蓝芯+红焰+青白边。
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

const SANMEI_RAMP: Array<[number, [number, number, number]]> = [
  [0.0, [210, 240, 255]], // 雪白蓝
  [0.25, [90, 180, 240]], // 冰蓝
  [0.55, [210, 90, 60]], // 朱砂
  [0.8, [140, 50, 25]], // 砖红
  [1.0, [60, 20, 25]], // 暗紫红
]

function ramp(age: number): [number, number, number] {
  for (let i = 1; i < SANMEI_RAMP.length; i++) {
    const stop = SANMEI_RAMP[i]!
    const prev = SANMEI_RAMP[i - 1]!
    if (age <= stop[0]) {
      const k = (age - prev[0]) / (stop[0] - prev[0])
      return [
        Math.round(prev[1][0] + (stop[1][0] - prev[1][0]) * k),
        Math.round(prev[1][1] + (stop[1][1] - prev[1][1]) * k),
        Math.round(prev[1][2] + (stop[1][2] - prev[1][2]) * k),
      ]
    }
  }
  return SANMEI_RAMP[SANMEI_RAMP.length - 1]![1]
}

const spawnAcc: number[] = []
let burstIn = 0

export const sanmei: FireEffect = {
  id: 'sanmei',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
    burstIn = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.ember < 0.01) return

    const baseRate = (budget.particles / 1.4) * gains.ember
    const winIdx = spawnAcc.length
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    spawnAcc[winIdx]! += baseRate * p.dt
    const steady = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= steady

    // 比 plume 更频繁的 burst（cool 调需要更多粒子衬出冷热对比）
    burstIn -= p.dt
    const bursting = burstIn <= 0
    if (bursting) burstIn = 0.7 + Math.random() * 0.5

    const total = steady + (bursting ? 4 + Math.floor(Math.random() * 4) : 0)
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 0.5 + Math.random() * 0.5
      const sizeBase = rect.ww * (0.035 + Math.random() * 0.045)
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.55,
        y: rect.wy + rect.wh * (0.78 + Math.random() * 0.18),
        vx: (Math.random() - 0.5) * rect.ww * 0.14,
        vy: -rect.wh * (1.4 + Math.random() * 1.0) * (bursting ? 1.6 : 1),
        life,
        maxLife: life,
        size: sizeBase,
        seed: Math.random() * Math.PI * 2,
        length: 1.5 + Math.random() * 0.9, // 比 plume 略长,带飘逸感
        wob: rect.ww * (0.05 + Math.random() * 0.05),
        asym: (Math.random() - 0.5) * 0.4,
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
      e.x += (e.vx + Math.sin(time * 4.5 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      e.vy *= 1 - 0.32 * dt
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    ctx.globalCompositeOperation = 'source-over'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      const lifeAlpha = Math.pow(1 - age, 1.3)
      if (lifeAlpha < 0.02) continue
      const w = e.size * (1 - age * 0.5)
      const h = w * e.length
      const [r, g, b] = ramp(age)

      // 三层叠加：内焰（蓝）→ 中焰（朱砂）→ 外焰（青白边）
      // 简化版：每个粒子先画主体（当前 ramp 色），再画一层细冷青光
      const cx = e.x + (e.asym ?? 0) * w * 0.5
      const cy = e.y - h * 0.5
      const grad = ctx.createLinearGradient(cx, cy + h * 0.5, cx, cy - h * 0.5)
      grad.addColorStop(0, `rgba(${r},${g},${b},${0.95 * lifeAlpha})`)
      grad.addColorStop(0.55, `rgba(${r},${g},${b},${0.55 * lifeAlpha})`)
      grad.addColorStop(1, `rgba(${r},${g},${b},0)`)
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.ellipse(cx, cy, w, h, 0, 0, Math.PI * 2)
      ctx.fill()

      // 冷青外焰：仅在年轻粒子（age < 0.4）上画一圈薄薄青白光
      if (age < 0.4) {
        const rimAlpha = (1 - age / 0.4) * 0.5 * lifeAlpha
        ctx.fillStyle = `rgba(180, 230, 255, ${rimAlpha})`
        ctx.beginPath()
        ctx.ellipse(cx, cy, w * 0.6, h * 0.7, 0, 0, Math.PI * 2)
        ctx.fill()
      }
    }
  },
}

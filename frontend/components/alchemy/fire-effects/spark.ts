/**
 * Spark 火花
 *
 * 金 + 朱砂二色火花颗粒，孔明灯升空感。
 *
 * 特点：
 *  - 不画"火苗"——只画无数小火花从底床上窜
 *  - 二色：金 (255, 215, 90) + 朱砂 (220, 80, 50) 随机混
 *  - 高 vy（比 plume 快 2x），像天灯上升
 *  - 短寿命（0.4-0.8s）但每秒喷发 80-100 粒 → 满屏金光
 *  - 粒径小（1-2% ww）但 alpha 高 → 像火星喷射
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

// 随机二选一
const SPARK_COLORS: Array<[number, number, number]> = [
  [255, 215, 90], // 金
  [255, 240, 180], // 浅金
  [220, 80, 50], // 朱砂
  [255, 130, 70], // 橙朱
]

const spawnAcc: number[] = []

export const spark: FireEffect = {
  id: 'spark',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.ember < 0.01) return
    // 喷发率高:每窗口 ~30/s
    const baseRate = (budget.particles / 0.6) * gains.ember
    const winIdx = spawnAcc.length
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    spawnAcc[winIdx]! += baseRate * p.dt
    const total = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= total
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 0.4 + Math.random() * 0.4
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.6,
        y: rect.wy + rect.wh * (0.85 + Math.random() * 0.13),
        vx: (Math.random() - 0.5) * rect.ww * 0.4,
        vy: -rect.wh * (3.0 + Math.random() * 2.5), // 快
        life,
        maxLife: life,
        size: rect.ww * (0.008 + Math.random() * 0.015), // 小
        seed: Math.random() * Math.PI * 2,
        color: Math.floor(Math.random() * SPARK_COLORS.length),
        wob: rect.ww * 0.1,
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
      // 慢速拖尾（buoyancy 衰减） + 横向轻飘
      e.x += (e.vx + Math.sin(time * 7 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      e.vy *= 1 - 0.4 * dt // 衰减更快，让火花"上窜"而非"飘升"
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    // spark 用 lighter 模式（亮点叠加）—— 满屏金光感
    ctx.globalCompositeOperation = 'lighter'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      const alpha = Math.pow(1 - age, 0.7) * 0.95
      if (alpha < 0.02) continue
      const colorIdx = (e.color ?? 0) | 0
      const [r, g, b] = SPARK_COLORS[colorIdx % SPARK_COLORS.length]!
      const radius = Math.max(0.3, e.size * (1 - age * 0.5))
      // 中心实色 + 外圈弱光晕
      const grad = ctx.createRadialGradient(e.x, e.y, 0, e.x, e.y, radius * 3)
      grad.addColorStop(0, `rgba(${r},${g},${b},${alpha})`)
      grad.addColorStop(0.3, `rgba(${r},${g},${b},${alpha * 0.6})`)
      grad.addColorStop(1, `rgba(${r},${g},${b},0)`)
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.arc(e.x, e.y, radius * 3, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

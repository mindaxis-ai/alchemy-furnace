/**
 * Ember 灯炭
 *
 * 改进版实色粒子：去掉米白起点 + 慢速 + source-over 混合。
 * 适合"火堆将熄"、"灯芯余烬"场景。安静、低调、不抢戏。
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

const EMBER_RAMP: Array<[number, [number, number, number]]> = [
  [0.0, [255, 150, 50]], // 橙（不再用米白）
  [0.4, [220, 90, 20]], // 深橙
  [0.75, [180, 50, 12]], // 砖红
  [1.0, [90, 22, 5]], // 暗红
]

function ramp(age: number): [number, number, number] {
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

export const ember: FireEffect = {
  id: 'ember',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.ember < 0.01) return
    // 慢：baseRate 用更小的分母（寿命长，所以同 rate 下 alive 数自然多，但
    // 这里想让画面"安静"，故直接压低 spawn rate）
    const baseRate = (budget.particles / 3.0) * gains.ember
    const winIdx = spawnAcc.length
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    spawnAcc[winIdx]! += baseRate * p.dt
    const total = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= total
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 1.2 + Math.random() * 0.8
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.7,
        y: rect.wy + rect.wh * (0.82 + Math.random() * 0.16),
        vx: (Math.random() - 0.5) * rect.ww * 0.08,
        vy: -rect.wh * (0.4 + Math.random() * 0.5),
        life,
        maxLife: life,
        size: rect.ww * (0.018 + Math.random() * 0.022),
        seed: Math.random() * Math.PI * 2,
        wob: rect.ww * 0.04,
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
      e.x += (e.vx + Math.sin(time * 3 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      e.vy *= 1 - 0.2 * dt
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    ctx.globalCompositeOperation = 'source-over'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      const alpha = Math.pow(1 - age, 1.0) * 0.85
      if (alpha < 0.02) continue
      const r = Math.max(0.3, e.size * (1 - age * 0.85))
      const [cr, cg, cb] = ramp(age)
      ctx.fillStyle = `rgba(${cr},${cg},${cb},${alpha})`
      ctx.beginPath()
      ctx.arc(e.x, e.y, r, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

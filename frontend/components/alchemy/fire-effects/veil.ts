/**
 * Veil 帐火
 *
 * 多层薄纱叠成大火焰轮廓。大羽化圆，3 层不同色 + 不同位置错位，
 * 用 'screen' 混合，叠出"大焰"感。少粒子但每粒都很大很柔。
 *
 * 适合"御火仪式"、"炼丹金光"、"三昧真火"等大场面。
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

// 三层薄纱：浅金 / 朱砂 / 砖红
const VEIL_LAYERS = [
  { color: [255, 230, 160], weight: 1.0 }, // 浅金
  { color: [220, 90, 50], weight: 0.7 }, // 朱砂
  { color: [120, 30, 18], weight: 0.5 }, // 砖红
] as const

const spawnAcc: number[] = []

export const veil: FireEffect = {
  id: 'veil',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.glow < 0.1) return
    // 少：每个窗口同时只 6-12 个 veil 粒子，靠层叠 + 大半径出效果
    const baseRate = (budget.particles / 4.0) * gains.glow
    const winIdx = spawnAcc.length
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    spawnAcc[winIdx]! += baseRate * p.dt
    const total = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= total
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 0.8 + Math.random() * 0.6
      // 起始 y 高一些（60-90% wh）—— 大焰从床层上方长出
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.5,
        y: rect.wy + rect.wh * (0.6 + Math.random() * 0.3),
        vx: (Math.random() - 0.5) * rect.ww * 0.05,
        vy: -rect.wh * (0.3 + Math.random() * 0.4),
        life,
        maxLife: life,
        // 大：10-20% ww 半径
        size: rect.ww * (0.1 + Math.random() * 0.1),
        seed: Math.random() * Math.PI * 2,
        wob: rect.ww * 0.08,
        // 出生时决定走哪一层
        layer: Math.floor(Math.random() * VEIL_LAYERS.length),
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
      e.x += (e.vx + Math.sin(time * 2 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      e.vy *= 1 - 0.25 * dt
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    // 帐火用 screen 模式：多色叠白 → 整体偏暖偏亮
    ctx.globalCompositeOperation = 'screen'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      // 透明度：升起 (0-0.2) 淡入，顶端 (0.7-1) 淡出
      const fadeIn = Math.min(1, age / 0.2)
      const fadeOut = 1 - Math.max(0, (age - 0.7) / 0.3)
      const alpha = Math.min(fadeIn, fadeOut) * 0.55
      if (alpha < 0.02) continue

      const layer = VEIL_LAYERS[Math.floor(e.layer ?? 0)] ?? VEIL_LAYERS[0]
      const w = e.size * (1 - age * 0.3)
      const [r, g, b] = layer.color
      const grad = ctx.createRadialGradient(e.x, e.y, 0, e.x, e.y, w)
      grad.addColorStop(
        0,
        `rgba(${r},${g},${b},${alpha * layer.weight})`
      )
      grad.addColorStop(
        0.5,
        `rgba(${r},${g},${b},${alpha * layer.weight * 0.5})`
      )
      grad.addColorStop(1, `rgba(${r},${g},${b},0)`)
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.arc(e.x, e.y, w, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

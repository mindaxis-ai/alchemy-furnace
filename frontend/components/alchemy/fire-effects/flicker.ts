/**
 * Flicker 灵火
 *
 * 1-3 帧爆发式闪烁簇。极少粒子，亮黄→红。每 60-180ms 一次爆闪，
 * 每次 1-3 个大椭圆 burst。同 burst 内粒子位置/大小接近，整团瞬间亮起瞬间灭。
 *
 * 适合"闪电"、"灵火"、"炼丹金光"等奇幻/仪式场景。
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

let burstIn = 0
let globalPhase = 0

export const flicker: FireEffect = {
  id: 'flicker',

  init(_ctx: EffectCtx) {
    burstIn = 0.05 + Math.random() * 0.1
    globalPhase = Math.random() * Math.PI * 2
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget } = p
    if (gains.glow < 0.05) return

    burstIn -= p.dt
    if (burstIn > 0) return

    // 一次新 burst
    burstIn = 0.06 + Math.random() * 0.12
    const size = 1 + Math.floor(Math.random() * 3) // 1~3 粒
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(size, room)

    for (let i = 0; i < k; i++) {
      const life = 0.06 + Math.random() * 0.09 // 1-3 帧
      // 整 burst 粒子位置相近（看动画 keytime 紧密）
      const cx = rect.wx + (Math.random() - 0.5) * rect.ww * 0.55
      const cy = rect.wy + rect.wh * (0.7 + Math.random() * 0.25)
      particles.push({
        x: cx + (Math.random() - 0.5) * rect.ww * 0.15,
        y: cy,
        vx: 0,
        vy: 0, // 静帧 burst，不动
        life,
        maxLife: life,
        size: rect.ww * (0.12 + Math.random() * 0.08),
        seed: Math.random() * Math.PI * 2,
        // 自定义：高亮强度（首帧最亮）
        flash: 1,
      })
    }
  },

  update(particles: Particle[], dt: number, _time: number, _p: FireParams) {
    for (let i = particles.length - 1; i >= 0; i--) {
      const e = particles[i]!
      e.life -= dt
      if (e.life <= 0) {
        particles.splice(i, 1)
      } else {
        // 亮度沿年龄快速衰减 (0.5+0.5*life/maxLife → 1 → 0)
        e.flash = 0.5 + 0.5 * (e.life / e.maxLife)
      }
    }
    globalPhase += dt
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    // flicker 用 lighter 模式（这是合适场景：短促爆闪 + 亮黄叠白）
    ctx.globalCompositeOperation = 'lighter'
    for (const e of particles) {
      const flash = e.flash ?? 1
      const w = e.size * flash
      const h = w * 0.6
      // 中心亮黄 (255,250,180) → 边缘红 (220,80,20) → 外圈透明
      const grad = ctx.createRadialGradient(e.x, e.y, 0, e.x, e.y, w)
      grad.addColorStop(0, `rgba(255,250,180,${0.95 * flash})`)
      grad.addColorStop(0.4, `rgba(255,160,40,${0.7 * flash})`)
      grad.addColorStop(1, 'rgba(220,80,20,0)')
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.ellipse(e.x, e.y, w, h, 0, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

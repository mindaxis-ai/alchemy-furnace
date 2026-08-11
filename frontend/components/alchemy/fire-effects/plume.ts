/**
 * Plume 火舌（默认）
 *
 * 锥形火苗 + 拖尾 + 黄→红渐变尖端。最像木柴/炉膛火。
 *
 * 视觉策略：
 *  - 每个粒子是一个 tapered ellipse（拖尾方向向上收尖）
 *  - 颜色沿 age 走 3 段：亮黄(0) → 橙(0.35) → 深红(1)，不掺白
 *  - alpha 沿 age = (1-age)^1.4，让火苗尖端更虚
 *  - 'source-over' 混合模式：饱和度保留，叠白水感消失
 *  - 喷发节奏：embark steady (1.4/spike/窗口/s) + 1.5s 间隔的 5 粒 burst
 */

import type { FireEffect, FireParams, Particle, EffectCtx } from './index'

const PLUME_RAMP: Array<[number, [number, number, number]]> = [
  [0.0, [255, 220, 120]], // 亮黄
  [0.35, [255, 150, 40]], // 橙
  [0.7, [220, 70, 20]], // 深橙红
  [1.0, [120, 28, 8]], // 余烬
]

function ramp(age: number): [number, number, number] {
  for (let i = 1; i < PLUME_RAMP.length; i++) {
    const stop = PLUME_RAMP[i]!
    const prev = PLUME_RAMP[i - 1]!
    if (age <= stop[0]) {
      const k = (age - prev[0]) / (stop[0] - prev[0])
      return [
        Math.round(prev[1][0] + (stop[1][0] - prev[1][0]) * k),
        Math.round(prev[1][1] + (stop[1][1] - prev[1][1]) * k),
        Math.round(prev[1][2] + (stop[1][2] - prev[1][2]) * k),
      ]
    }
  }
  return PLUME_RAMP[PLUME_RAMP.length - 1]![1]
}

const spawnAcc: number[] = []
let burstIn = 0

export const plume: FireEffect = {
  id: 'plume',

  init(_ctx: EffectCtx) {
    spawnAcc.length = 0
    burstIn = 0
  },

  spawn(particles: Particle[], p: FireParams) {
    const { rect, gains, budget, time } = p
    if (gains.ember < 0.01) return

    // steady 喷发率 (粒子/秒/窗口)，按 ember stage 与 budget.cap 平滑
    const baseRate = (budget.particles / 1.6) * gains.ember
    const winIdx = spawnAcc.length // 简单按调用顺序累
    if (spawnAcc.length <= winIdx) spawnAcc.push(0)
    spawnAcc[winIdx]! += baseRate * p.dt
    const steady = Math.floor(spawnAcc[winIdx]!)
    spawnAcc[winIdx]! -= steady

    // 间隔 burst：每 1.5s 一次 5 粒小爆发
    burstIn -= p.dt
    const bursting = burstIn <= 0
    if (bursting) burstIn = 1.0 + Math.random() * 0.8

    const total = steady + (bursting ? 3 + Math.floor(Math.random() * 4) : 0)
    const room = Math.max(0, budget.particles - particles.length)
    const k = Math.min(total, room)

    for (let i = 0; i < k; i++) {
      const life = 0.55 + Math.random() * 0.55
      const sizeBase = rect.ww * (0.04 + Math.random() * 0.05)
      particles.push({
        x: rect.wx + (Math.random() - 0.5) * rect.ww * 0.6,
        // 略高于底床（95%）让火苗从床层表面长出
        y: rect.wy + rect.wh * (0.78 + Math.random() * 0.18),
        vx: (Math.random() - 0.5) * rect.ww * 0.18,
        // 强上升，burst 粒子再快 50%
        vy: -rect.wh * (1.6 + Math.random() * 1.0) * (bursting ? 1.5 : 1),
        life,
        maxLife: life,
        size: sizeBase,
        seed: Math.random() * Math.PI * 2,
        // 自定义：拖尾高度（粒子身长 = size * length），沿 age 收尖
        length: 1.4 + Math.random() * 0.8,
        // 横向呼吸幅度（弱，不像水波）
        wob: rect.ww * (0.06 + Math.random() * 0.06),
        // 出生时的随机扰动，让火苗不对称
        asym: (Math.random() - 0.5) * 0.4,
        // 时间戳用于呼吸
        born: time,
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
      const age = 1 - e.life / e.maxLife
      // 弱横向呼吸（火苗左右轻摆），比 water 版的振幅小很多
      e.x += (e.vx + Math.sin(time * 5 + e.seed) * e.wob) * dt
      e.y += e.vy * dt
      // 越往上越慢（buoyancy 衰减），火苗尖端更虚
      e.vy *= 1 - 0.3 * dt
    }
  },

  draw(ctx: CanvasRenderingContext2D, particles: Particle[], _p: FireParams) {
    ctx.globalCompositeOperation = 'source-over'
    for (const e of particles) {
      const age = 1 - e.life / e.maxLife
      // 年轻 = 大 + 不透明；衰老 = 收尖 + 渐隐
      const lifeAlpha = Math.pow(1 - age, 1.4)
      if (lifeAlpha < 0.02) continue

      // 沿 vy 方向画 tapered ellipse：
      //  - 主体在 (x, y)，向上延伸 length 倍 size
      //  - 尾端在 (x - vyDir*len*size, y - len*size) 附近
      const w = e.size * (1 - age * 0.55)
      const h = w * e.length
      const [r, g, b] = ramp(age)

      // 渐变：底（粒子中心）= 当前色；顶 = 同色 alpha 0
      // 用粒子中心作中心 (cx,cy)，旋转 0（火苗垂直向上）
      const cx = e.x + (e.asym ?? 0) * w * 0.6
      const cy = e.y - h * 0.5
      const grad = ctx.createLinearGradient(cx, cy + h * 0.5, cx, cy - h * 0.5)
      grad.addColorStop(0, `rgba(${r},${g},${b},${0.95 * lifeAlpha})`)
      grad.addColorStop(0.5, `rgba(${r},${g},${b},${0.6 * lifeAlpha})`)
      grad.addColorStop(1, `rgba(${r},${g},${b},0)`)
      ctx.fillStyle = grad

      ctx.beginPath()
      // ellipse 中心 (cx, cy)，半径 (w, h)
      ctx.ellipse(cx, cy, w, h, 0, 0, Math.PI * 2)
      ctx.fill()
    }
  },
}

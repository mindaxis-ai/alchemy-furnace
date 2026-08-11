'use client'

/**
 * 设置页「炉火」tab 的 mini 实时预览
 *
 * 不用真 BaguaFurnace（那张图要 240x80 太脆），自己画一个极简版：
 *  - 单个 arch 窗口（240x80 视口内居中）
 *  - 永远 intensity=1（强制 ramp 到 1）
 *  - 火走 effect 的 spawn/update/draw
 *  - 烟走自己的简化版：白点上升 + 横向轻飘（不调 smoke.tsx，因为它要 staged ignition + 复杂路径）
 *
 * 切换 effectId / smokeLevel 时：清掉粒子池，让 ramp 重新来过。
 */

import { useEffect, useRef } from 'react'
import {
  EFFECTS,
  type FireEffectId,
  type Particle,
} from './fire-effects'

const PREVIEW_W = 240
const PREVIEW_H = 80

/** smoke mini：单点上升 + 横向轻飘。level 控制喷发率与最大密度。 */
interface MiniSmoke {
  x: number
  y: number
  vx: number
  vy: number
  life: number
  maxLife: number
  size: number
  seed: number
}

function drawPreview(
  ctx: CanvasRenderingContext2D,
  particles: Particle[],
  smoke: MiniSmoke[],
  effectId: FireEffectId,
  smokeLevel: number,
  prog: number
) {
  ctx.clearRect(0, 0, PREVIEW_W, PREVIEW_H)

  // 假鼎边框（暗铜色细线，让预览像炉口）
  ctx.save()
  ctx.strokeStyle = 'rgba(120, 90, 60, 0.5)'
  ctx.lineWidth = 1
  ctx.strokeRect(0.5, 0.5, PREVIEW_W - 1, PREVIEW_H - 1)

  // 暗腔
  const cx = PREVIEW_W / 2
  const cy = PREVIEW_H * 0.6
  const r = Math.min(PREVIEW_W, PREVIEW_H) * 0.4
  const cavity = ctx.createRadialGradient(cx, cy, 0, cx, cy, r)
  cavity.addColorStop(0, 'rgba(25, 8, 3, 0.88)')
  cavity.addColorStop(0.7, 'rgba(45, 16, 6, 0.6)')
  cavity.addColorStop(1, 'rgba(70, 28, 10, 0)')
  ctx.fillStyle = cavity
  // arch 窗口裁剪：半圆顶 + 直边
  const arcR = PREVIEW_W * 0.42
  const arcCx = PREVIEW_W / 2
  const arcCy = PREVIEW_H * 0.45
  ctx.beginPath()
  ctx.moveTo(arcCx - arcR, arcCy)
  ctx.arc(arcCx, arcCy, arcR, Math.PI, 0)
  ctx.lineTo(arcCx + arcR, PREVIEW_H)
  ctx.lineTo(arcCx - arcR, PREVIEW_H)
  ctx.closePath()
  ctx.fill()
  ctx.restore()

  // 粒子
  const effect = EFFECTS[effectId]
  const rect = {
    wx: arcCx,
    wy: arcCy,
    ww: arcR * 2,
    wh: PREVIEW_H - arcCy,
  }
  effect.draw(ctx, particles, {
    win: { id: 'preview', x: 50, width: 84, top: 56, height: 53, phase: 0 },
    rect,
    dt: 0.016,
    intensity: prog,
    gains: { warm: 1, bed: 1, glow: 1, ember: prog },
    budget: { particles: 60, glow: true },
    pixelRatio: 1,
    time: 0,
    seed: 0,
  })

  // 简化烟
  if (smokeLevel > 0.01) {
    ctx.globalCompositeOperation = 'source-over'
    for (const w of smoke) {
      const age = 1 - w.life / w.maxLife
      const alpha = Math.min(1, age / 0.2) * (1 - Math.max(0, (age - 0.7) / 0.3)) * 0.55
      if (alpha < 0.02) continue
      const grad = ctx.createRadialGradient(w.x, w.y, 0, w.x, w.y, w.size)
      grad.addColorStop(0, `rgba(150, 156, 160, ${alpha})`)
      grad.addColorStop(1, 'rgba(120, 126, 130, 0)')
      ctx.fillStyle = grad
      ctx.beginPath()
      ctx.arc(w.x, w.y, w.size, 0, Math.PI * 2)
      ctx.fill()
    }
  }
}

export function FireEffectPreview({
  effectId,
  smokeLevel,
}: {
  effectId: FireEffectId
  smokeLevel: number
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const particlesRef = useRef<Particle[]>([])
  const smokeRef = useRef<MiniSmoke[]>([])
  const progRef = useRef(0)
  const lastTsRef = useRef(0)

  // 切换 effect 时清空粒子 + 重新 ramp；smokeLevel 变化不重置已有烟雾
  useEffect(() => {
    particlesRef.current = []
    progRef.current = 0
    smokeRef.current = []
    EFFECTS[effectId].init?.({ pixelRatio: 1, budget: { particles: 60, glow: true } })
  }, [effectId])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    canvas.width = PREVIEW_W
    canvas.height = PREVIEW_H

    let raf = 0
    const arcCx = PREVIEW_W / 2
    const arcR = PREVIEW_W * 0.42
    const arcCy = PREVIEW_H * 0.45

    const loop = (ts: number) => {
      const dt = Math.min(0.05, lastTsRef.current ? (ts - lastTsRef.current) / 1000 : 0.016)
      const time = ts / 1000
      lastTsRef.current = ts

      // 永远 ramp 到 1（约 0.6s）
      progRef.current = Math.min(1, progRef.current + dt / 0.6)
      const prog = progRef.current

      const effect = EFFECTS[effectId]
      const rect = {
        wx: arcCx,
        wy: arcCy,
        ww: arcR * 2,
        wh: PREVIEW_H - arcCy,
      }
      const params = {
        win: { id: 'preview', x: 50, width: 84, top: 56, height: 53, phase: 0 },
        rect,
        dt,
        intensity: prog,
        gains: { warm: 1, bed: 1, glow: prog, ember: prog },
        budget: { particles: 60, glow: true },
        pixelRatio: 1,
        time,
        seed: 0,
      }
      effect.spawn(particlesRef.current, params)
      effect.update(particlesRef.current, dt, time, params)

      // mini smoke
      const target = 18 * smokeLevel * dt // 0..18 /s
      const count = Math.floor(target) + (Math.random() < target % 1 ? 1 : 0)
      const maxWisps = Math.floor(50 * smokeLevel)
      for (let i = 0; i < count && smokeRef.current.length < maxWisps; i++) {
        const life = 1.4 + Math.random() * 1.0
        smokeRef.current.push({
          x: arcCx + (Math.random() - 0.5) * arcR * 1.4,
          y: arcCy + (PREVIEW_H - arcCy) * 0.9 - Math.random() * 6,
          vx: (Math.random() - 0.5) * 8,
          vy: -12 - Math.random() * 8,
          life,
          maxLife: life,
          size: 6 + Math.random() * 5,
          seed: Math.random() * Math.PI * 2,
        })
      }
      for (let i = smokeRef.current.length - 1; i >= 0; i--) {
        const w = smokeRef.current[i]!
        w.life -= dt
        if (w.life <= 0) {
          smokeRef.current.splice(i, 1)
          continue
        }
        const age = 1 - w.life / w.maxLife
        w.x += (w.vx + Math.sin(time * 2 + w.seed) * 5) * dt
        w.y += w.vy * dt
        w.size += dt * 1.5
      }

      drawPreview(ctx, particlesRef.current, smokeRef.current, effectId, smokeLevel, prog)
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    return () => cancelAnimationFrame(raf)
  }, [effectId, smokeLevel])

  return (
    <div className="flex justify-center">
      <canvas
        ref={canvasRef}
        aria-label="炉火预览"
        className="rounded-xl border border-border/60 bg-paper-deep"
        style={{ width: PREVIEW_W, height: PREVIEW_H }}
      />
    </div>
  )
}

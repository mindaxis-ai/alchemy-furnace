'use client'

import { useEffect, useRef } from 'react'
import {
  DEFAULT_EFFECT_ID,
  EFFECTS,
  type FireEffectId,
  type Particle,
  type WinRect,
  type StageGains,
} from './fire-effects'
import {
  advanceIgnition,
  archPath,
  drawBedAndGlow,
  drawCavity,
  smoothstep,
  stageGains,
  windowRect as rectOf,
} from './fire-runtime'

export interface FurnaceWindow {
  id: string
  x: number // horizontal center, % of image width
  width: number // % of image width
  top: number // top edge, % of image height
  height: number // % of image height
  phase: number // animation phase offset, seconds
}

interface FireBudget {
  particles: number
  glow: boolean
}

export interface BaguaFurnaceFireProps {
  windows: FurnaceWindow[]
  intensity: number // 0..1
  budget: FireBudget
  pixelRatio: number
  paused: boolean
  effectId?: FireEffectId
}

/**
 * Staged ignition clock (shared by fire and smoke): ramps 0→1 over ~0.6s on
 * hover-in and 1→0 over ~0.7s on hover-out. Each effect layer eases in over
 * its own threshold range of the clock, so the furnace catches fire in quick
 * succession: cavity warms → ember bed breathes → glow fills → sparks rise.
 */
export const IGNITE_S = 0.6
export const EXTINGUISH_S = 0.7

export { advanceIgnition, smoothstep }

/**
 * Fire 调度器（006-fire）
 *
 * 调度器只管：窗口裁剪 + cavity 暗腔铺底 + staged ignition 推进 + staged gains。
 * 粒子怎么喷、怎么飞、怎么画，全部转交给当前 effect。
 *
 * 切换效果：改 DEFAULT_EFFECT_ID（'plume' | 'ember' | 'flicker' | 'veil'）。
 */
export function BaguaFurnaceFire({
  windows,
  intensity,
  budget,
  pixelRatio,
  paused,
  effectId,
}: BaguaFurnaceFireProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)
  const particlesRef = useRef<Particle[]>([])
  const progRef = useRef(0) // staged ignition clock 0..1
  const rafRef = useRef<number>(0)
  const lastTsRef = useRef<number>(0)
  const sizeRef = useRef({ w: 0, h: 0 })

  // 当前 effect：硬编码默认 plume，dev 改 EFFECT_ID 验证调度器
  const effect = EFFECTS[effectId ?? DEFAULT_EFFECT_ID]

  useEffect(() => {
    const canvas = canvasRef.current
    const wrap = wrapRef.current
    if (!canvas || !wrap) return

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // Use clientWidth/clientHeight (layout size), NOT getBoundingClientRect:
    // an ancestor has transform: scale(...), and the rect would bake that
    // scale into the canvas pixel size, double-scaling it against the image.
    const resize = () => {
      const w = Math.max(1, wrap.clientWidth)
      const h = Math.max(1, wrap.clientHeight)
      sizeRef.current = { w, h }
      canvas.width = Math.floor(w * pixelRatio)
      canvas.height = Math.floor(h * pixelRatio)
      ctx.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
    }

    resize()
    const ro = new ResizeObserver(resize)
    ro.observe(wrap)

    // Init current effect
    effect.init?.({ pixelRatio, budget })

    const windowRect = (win: FurnaceWindow): WinRect => {
      const { w, h } = sizeRef.current
      return {
        wx: (win.x / 100) * w,
        wy: (win.top / 100) * h,
        ww: (win.width / 100) * w,
        wh: (win.height / 100) * h,
      }
    }

    // Arch window shape, matching the SVG cut-out mask: semicircle top
    // (radius = half window width), straight sides, straight bottom.
    const drawWindowClip = () => {
      ctx.beginPath()
      for (const win of windows) {
        const { wx, wy, ww, wh } = windowRect(win)
        const r = ww / 2
        ctx.moveTo(wx - r, wy + r)
        ctx.arc(wx, wy + r, r, Math.PI, 0)
        ctx.lineTo(wx + r, wy + wh)
        ctx.lineTo(wx - r, wy + wh)
        ctx.closePath()
      }
      ctx.clip()
    }

    // 暗腔：每个窗口一个深色径向渐变（黑底 + 中心微亮），保证 cut-out 透出
    // 来的不是纯白页背景。永远画（哪怕 prog=0，cut-out 也得有底色）。
    const drawCavity = () => {
      for (const win of windows) {
        const { wx, wy, ww, wh } = windowRect(win)
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

    // bed 呼吸 + glow 暖光：仅在预算允许时画
    const drawBedAndGlow = (gains: StageGains) => {
      if (gains.bed > 0.01) {
        for (const win of windows) {
          const { wx, wy, ww, wh } = windowRect(win)
          const pulse =
            0.88 +
            0.12 *
              Math.sin((lastTsRef.current / 1000) * 7 + win.phase * 6.3) *
              Math.sin((lastTsRef.current / 1000) * 2.3 + win.phase * 12.6)
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
          const { wx, wy, ww, wh } = windowRect(win)
          const time = lastTsRef.current / 1000
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

    const loop = (ts: number) => {
      if (paused) {
        rafRef.current = requestAnimationFrame(loop)
        return
      }
      const dt = Math.min(0.05, lastTsRef.current ? (ts - lastTsRef.current) / 1000 : 0.016)
      const time = ts / 1000
      lastTsRef.current = ts

      // Advance the staged ignition clock and derive per-layer gains.
      const prog = (progRef.current = advanceIgnition(progRef.current, intensity, dt))
      const gains: StageGains = {
        warm: smoothstep(0.0, 0.35, prog),
        bed: smoothstep(0.15, 0.55, prog),
        glow: smoothstep(0.35, 0.8, prog),
        ember: smoothstep(0.5, 0.85, prog),
      }

      // Spawn: dispatch to current effect, once per window
      for (let i = 0; i < windows.length; i++) {
        const win = windows[i]!
        const rect = windowRect(win)
        effect.spawn(particlesRef.current, {
          win,
          rect,
          dt,
          intensity: prog,
          gains,
          budget,
          pixelRatio,
          time,
          seed: win.phase,
        })
      }

      // Update: effect drives its own particle kinematics
      effect.update(particlesRef.current, dt, time, {
        win: windows[0]!,
        rect: windowRect(windows[0]!),
        dt,
        intensity: prog,
        gains,
        budget,
        pixelRatio,
        time,
        seed: 0,
      })

      // Draw
      const { w, h } = sizeRef.current
      ctx.clearRect(0, 0, w, h)

      ctx.save()
      drawWindowClip()
      drawCavity()
      // 床层呼吸 + 暖光（调度器画，effect 不管）
      drawBedAndGlow(gains)
      // 粒子层：effect 自己画
      effect.draw(ctx, particlesRef.current, {
        win: windows[0]!,
        rect: windowRect(windows[0]!),
        dt,
        intensity: prog,
        gains,
        budget,
        pixelRatio,
        time,
        seed: 0,
      })
      ctx.restore()

      rafRef.current = requestAnimationFrame(loop)
    }

    rafRef.current = requestAnimationFrame(loop)

    return () => {
      cancelAnimationFrame(rafRef.current)
      ro.disconnect()
    }
  }, [windows, intensity, budget, pixelRatio, paused, effect])

  return (
    <div ref={wrapRef} className="pointer-events-none absolute inset-0">
      <canvas
        ref={canvasRef}
        aria-hidden
        className="block h-full w-full"
      />
    </div>
  )
}

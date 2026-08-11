'use client'

import { useEffect, useRef } from 'react'

export interface FurnaceWindow {
  id: string
  x: number // horizontal center, % of image width
  width: number // % of image width
  top: number // top edge, % of image height
  height: number // % of image height
  phase: number // animation phase offset, seconds
}

interface Ember {
  x: number
  y: number
  vx: number
  vy: number
  life: number
  maxLife: number
  size: number // radius (px) at birth
  wob: number // horizontal wobble velocity amplitude (px/s)
  seed: number // wobble phase
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
}

/**
 * Ember cooling ramp by age (0 = just born, 1 = dead):
 * white-hot → amber → orange → orange-red → deep red.
 */
const EMBER_RAMP: Array<[number, [number, number, number]]> = [
  [0.0, [255, 246, 216]],
  [0.25, [255, 217, 122]],
  [0.55, [255, 154, 60]],
  [0.8, [224, 85, 26]],
  [1.0, [140, 34, 8]],
]

/** smoothstep between edge a and b */
export function smoothstep(a: number, b: number, x: number): number {
  const t = Math.min(1, Math.max(0, (x - a) / (b - a)))
  return t * t * (3 - 2 * t)
}

/**
 * Staged ignition clock (shared by fire and smoke): ramps 0→1 over ~1.1s on
 * hover-in and 1→0 over ~1.0s on hover-out. Each effect layer eases in over
 * its own threshold range of the clock, so the furnace catches fire in quick
 * succession: cavity warms → ember bed breathes → glow fills → sparks rise →
 * smoke last.
 */
const IGNITE_S = 0.6
const EXTINGUISH_S = 0.7

export function advanceIgnition(prog: number, target: number, dt: number): number {
  const dir = target > prog ? 1 : -1
  const dur = dir > 0 ? IGNITE_S : EXTINGUISH_S
  return Math.min(1, Math.max(0, prog + (dir * dt) / dur))
}

function emberColor(age: number): [number, number, number] {
  for (let i = 1; i < EMBER_RAMP.length; i++) {
    const stop = EMBER_RAMP[i]!
    const prev = EMBER_RAMP[i - 1]!
    if (age <= stop[0]) {
      const k = (age - prev[0]) / (stop[0] - prev[0])
      const c0 = prev[1]
      const c1 = stop[1]
      return [
        Math.round(c0[0] + (c1[0] - c0[0]) * k),
        Math.round(c0[1] + (c1[1] - c0[1]) * k),
        Math.round(c0[2] + (c1[2] - c0[2]) * k),
      ]
    }
  }
  return EMBER_RAMP[EMBER_RAMP.length - 1]![1]
}

export function BaguaFurnaceFire({
  windows,
  intensity,
  budget,
  pixelRatio,
  paused,
}: BaguaFurnaceFireProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)
  const embersRef = useRef<Ember[]>([])
  const progRef = useRef(0) // staged ignition clock 0..1
  const rafRef = useRef<number>(0)
  const lastTsRef = useRef<number>(0)
  const sizeRef = useRef({ w: 0, h: 0 })

  useEffect(() => {
    const canvas = canvasRef.current
    const wrap = wrapRef.current
    if (!canvas || !wrap) return

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // Use clientWidth/clientHeight (layout size), NOT getBoundingClientRect:
    // an ancestor has transform: scale(1.1), and the rect would bake that
    // scale into the canvas pixel size, double-scaling it against the image.
    // Layout size is also what the element's own CSS (h-full w-full) uses.
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

    const windowRect = (win: FurnaceWindow) => {
      const { w, h } = sizeRef.current
      return {
        wx: (win.x / 100) * w,
        wy: (win.top / 100) * h,
        ww: (win.width / 100) * w,
        wh: (win.height / 100) * h,
      }
    }

    // Embers are born on the ember bed (bottom ~15-20% of the window) and
    // rise across the window in ~0.4-0.9s. `boost` > 1 marks burst particles.
    const spawnEmber = (win: FurnaceWindow, boost: number) => {
      const { wx, wy, ww, wh } = windowRect(win)
      const life = 0.32 + Math.random() * 0.45
      embersRef.current.push({
        x: wx + (Math.random() - 0.5) * ww * 0.72,
        y: wy + wh * (0.82 + Math.random() * 0.16),
        vx: (Math.random() - 0.5) * ww * 0.5,
        vy: -wh * (2.2 + Math.random() * 2.0) * boost,
        life,
        maxLife: life,
        size: ww * (0.015 + Math.random() * 0.03),
        wob: ww * (0.3 + Math.random() * 0.4),
        seed: Math.random() * Math.PI * 2,
      })
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

    // Per-window steady-spawn accumulator and burst countdown (2-4s).
    const spawnAcc = windows.map(() => 0)
    const burstIn = windows.map(() => 1 + Math.random() * 3)

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
      const warmGain = smoothstep(0.0, 0.35, prog)   // cavity turns warm
      const bedGain = smoothstep(0.15, 0.55, prog)   // ember bed breathes
      const glowGain = smoothstep(0.35, 0.8, prog)   // glow fills the window
      const emberGain = smoothstep(0.5, 0.85, prog)  // sparks rise

      // Spawn steady embers + occasional burst clusters while lit. Rate is
      // derived from the particle cap: alive ≈ rate × avg life (0.85s).
      if (emberGain > 0.01) {
        const rate = (budget.particles / (0.85 * windows.length)) * emberGain
        windows.forEach((win, i) => {
          spawnAcc[i]! += rate * dt
          const steady = Math.floor(spawnAcc[i]!)
          spawnAcc[i]! -= steady

          burstIn[i]! -= dt
          const bursting = burstIn[i]! <= 0
          if (bursting) burstIn[i]! = 2 + Math.random() * 2

          const total = steady + (bursting ? 8 + Math.floor(Math.random() * 9) : 0)
          const room = Math.max(0, budget.particles - embersRef.current.length)
          for (let k = 0; k < Math.min(total, room); k++) {
            spawnEmber(win, bursting ? 1.5 : 1)
          }
        })
      }

      for (const e of embersRef.current) {
        e.life -= dt
        e.x += (e.vx + Math.sin(time * 6 + e.seed) * e.wob) * dt
        e.y += e.vy * dt
        e.vy *= 1 - 0.12 * dt // slight drag as the ember cools
      }
      embersRef.current = embersRef.current.filter((e) => e.life > 0)

      const { w, h } = sizeRef.current
      ctx.clearRect(0, 0, w, h)

      ctx.save()
      drawWindowClip()

      // dark furnace mouth — ALWAYS drawn: the cut-out ding layer has the
      // windows removed, and during the lit/unlit cross-fade the unmodified
      // image layer is semi-transparent, so without this the page background
      // (white) would show through. At full rest the unmodified image layer
      // covers it completely. Fire is layered on top when lit.
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

      if (prog > 0.01) {
        ctx.globalCompositeOperation = 'lighter'

        // stage 1: the cavity warms up — faint heat wash inside each window
        if (warmGain > 0.01) {
          for (const win of windows) {
            const { wx, wy, ww, wh } = windowRect(win)
            const g = ctx.createRadialGradient(
              wx,
              wy + wh * 0.6,
              0,
              wx,
              wy + wh * 0.6,
              Math.max(ww, wh) * 0.7
            )
            g.addColorStop(0, `rgba(200, 80, 25, ${0.2 * warmGain})`)
            g.addColorStop(0.6, `rgba(150, 45, 10, ${0.12 * warmGain})`)
            g.addColorStop(1, 'rgba(100, 25, 5, 0)')
            ctx.fillStyle = g
            ctx.fillRect(wx - ww / 2, wy, ww, wh)
          }
        }

        // stage 2: ember bed at the bottom of each window, breathing at ~1-2Hz
        if (bedGain > 0.01) {
          for (const win of windows) {
            const { wx, wy, ww, wh } = windowRect(win)
            const pulse =
              0.88 +
              0.12 *
                Math.sin(time * 7 + win.phase * 6.3) *
                Math.sin(time * 2.3 + win.phase * 12.6)
            const p = pulse * bedGain
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
            ctx.ellipse(wx, wy + wh * 0.94, ww * 0.50, wh * 0.18, 0, 0, Math.PI * 2)
            ctx.fill()
          }
        }

        // stage 3: base furnace glow filling each window: bright warm core
        // slightly below center, cooling to deep red at the edges
        if (glowGain > 0.01) {
          for (const win of windows) {
            const { wx, wy, ww, wh } = windowRect(win)
            const flicker = (0.92 + 0.08 * Math.sin(time * 11 + win.phase * 9)) * glowGain
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

        // stage 4: rising embers
        if (emberGain > 0.01) {
          for (const e of embersRef.current) {
            const age = 1 - e.life / e.maxLife
            const [r, g, b] = emberColor(age)
            const alpha = Math.pow(1 - age, 1.1) * 0.9 * emberGain
            const radius = Math.max(0.3, e.size * (1 - age * 0.9))
            ctx.fillStyle = `rgba(${r}, ${g}, ${b}, ${alpha})`
            ctx.beginPath()
            ctx.arc(e.x, e.y, radius, 0, Math.PI * 2)
            ctx.fill()
          }
        }
      }
      ctx.restore()
      rafRef.current = requestAnimationFrame(loop)
    }

    rafRef.current = requestAnimationFrame(loop)

    return () => {
      cancelAnimationFrame(rafRef.current)
      ro.disconnect()
    }
  }, [windows, intensity, budget.particles, pixelRatio, paused])

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

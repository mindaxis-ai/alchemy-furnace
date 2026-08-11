'use client'

import { useEffect, useRef } from 'react'
import { advanceIgnition, smoothstep, type FurnaceWindow } from './bagua-furnace-fire'

interface Wisp {
  x: number
  y: number
  size: number
  life: number
  maxLife: number
  seed: number
  drift: number
  rise: number // base climb speed, px/s (proportional to container height)
}

interface SmokeBudget {
  wisps: number
}

export interface BaguaFurnaceSmokeProps {
  windows: FurnaceWindow[]
  intensity: number // 0..1
  budget: SmokeBudget
  pixelRatio: number
  paused: boolean
  /** 用户烟雾浓度 0..1，默认 1。level=0 时不再 spawn 新烟，存量自然消散。 */
  level?: number
}

export function BaguaFurnaceSmoke({
  windows,
  intensity,
  budget,
  pixelRatio,
  paused,
  level = 1,
}: BaguaFurnaceSmokeProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)
  const wispsRef = useRef<Wisp[]>([])
  const progRef = useRef(0) // staged ignition clock 0..1 (smoke starts last)
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

    const spawn = (count: number) => {
      const { w, h } = sizeRef.current
      for (let i = 0; i < count; i++) {
        const win = windows[Math.floor(Math.random() * windows.length)]
        const wx = (win.x / 100) * w
        const wy = (win.top / 100) * h
        const ww = (win.width / 100) * w
        // 袅袅青烟: thin delicate wisps, not billowing clouds
        const size = ww * (0.25 + Math.random() * 0.35)
        // 寿命缩短:浓度高时堆积更明显(同时间窗内更多粒子)
        const life = 3.0 + Math.random() * 1.5
        wispsRef.current.push({
          x: wx + (Math.random() - 0.5) * ww * 0.9,
          y: wy + size * 0.35 - Math.random() * 14,
          size,
          life,
          maxLife: life,
          seed: Math.random() * Math.PI * 2,
          drift: (Math.random() - 0.5) * 70,
          rise: h * (0.065 + Math.random() * 0.03),
        })
      }
    }

    const loop = (ts: number) => {
      if (paused) {
        rafRef.current = requestAnimationFrame(loop)
        return
      }
      const dt = Math.min(0.05, lastTsRef.current ? (ts - lastTsRef.current) / 1000 : 0.016)
      lastTsRef.current = ts

      // Ignition clock shared with the fire; smoke only joins near full burn.
      const prog = (progRef.current = advanceIgnition(progRef.current, intensity, dt))
      // 喷发率 10 → 30/秒:让 level 变化时密度差更明显
      const spawnRate = 30 * level * smoothstep(0.75, 1.0, prog)

      // spawn
      const target = spawnRate * dt
      const count = Math.floor(target) + (Math.random() < target % 1 ? 1 : 0)
      // sqrt 曲线:低端更敏感 (level=0.5 → ~78% 上限;线性只 50%)
      const effectiveBudget = Math.max(0, Math.floor(budget.wisps * Math.sqrt(level)))
      if (count > 0 && wispsRef.current.length < effectiveBudget) {
        spawn(Math.min(count, effectiveBudget - wispsRef.current.length))
      }

      // update — the climb accelerates slightly with altitude (buoyant plume
      // entrainment), so smoke keeps rising past the furnace crown instead of
      // stalling at the vents
      for (const w of wispsRef.current) {
        w.life -= dt
        const age = 1 - w.life / w.maxLife
        w.y -= w.rise * (0.7 + age * 0.9) * dt
        w.x += (Math.sin(age * 2.2 + w.seed) * 18 + Math.sin(age * 5 + w.seed * 2) * 9 + w.drift * 0.22) * dt
        w.size += dt * 5
      }
      wispsRef.current = wispsRef.current.filter((w) => w.life > 0)

      // draw
      const { w, h } = sizeRef.current
      ctx.clearRect(0, 0, w, h)

      // Lingering wisps finish their lives even after intensity drops to 0
      // (hover-out), so the smoke dissipates instead of vanishing mid-air.
      if (wispsRef.current.length === 0) {
        rafRef.current = requestAnimationFrame(loop)
        return
      }

      for (const w of wispsRef.current) {
        const age = 1 - w.life / w.maxLife
        // Fade in quickly off the vent, stay visible through the climb, and
        // only dissipate near the end of the rise (previous 1-age² envelope
        // faded mid-body, which made smoke look parked at the vents).
        const alpha = 0.2 * smoothstep(0, 0.12, age) * (1 - smoothstep(0.55, 1, age))
        const px = w.x
        const py = w.y
        const r = w.size

        ctx.shadowBlur = 0

        // cool blue-grey 青灰, not warm chimney grey
        const g = ctx.createRadialGradient(px, py, 0, px, py, r)
        g.addColorStop(0, `rgba(150, 156, 160, ${alpha})`)
        g.addColorStop(0.45, `rgba(128, 134, 138, ${alpha * 0.55})`)
        g.addColorStop(1, 'rgba(105, 110, 114, 0)')
        ctx.fillStyle = g
        ctx.beginPath()
        ctx.arc(px, py, r, 0, Math.PI * 2)
        ctx.fill()
      }

      rafRef.current = requestAnimationFrame(loop)
    }

    rafRef.current = requestAnimationFrame(loop)

    return () => {
      cancelAnimationFrame(rafRef.current)
      ro.disconnect()
    }
  }, [windows, intensity, budget.wisps, pixelRatio, paused])

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

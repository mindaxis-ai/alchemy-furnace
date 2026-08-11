'use client'

import { useEffect, useRef } from 'react'
import { advanceIgnition } from './fire-runtime'
import type { FurnaceWindow } from './bagua-furnace-fire'
import {
  drawWisps,
  shouldSpawn,
  smokeBudget,
  spawnWisps,
  updateWisps,
  type Wisp,
} from './smoke-runtime'

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

    const loop = (ts: number) => {
      if (paused) {
        rafRef.current = requestAnimationFrame(loop)
        return
      }
      const dt = Math.min(0.05, lastTsRef.current ? (ts - lastTsRef.current) / 1000 : 0.016)
      lastTsRef.current = ts

      // Ignition clock shared with the fire; smoke only joins near full burn.
      const prog = (progRef.current = advanceIgnition(progRef.current, intensity, dt))

      // spawn（用 runtime 共享：40*level^1.5 喷发率 + 1.5 曲线 budget）
      const effectiveBudget = smokeBudget(level)
      const count = shouldSpawn(prog, level, wispsRef.current.length, effectiveBudget, dt)
      if (count > 0) {
        spawnWisps(wispsRef.current, count, windows, sizeRef.current)
      }

      // update
      updateWisps(wispsRef.current, dt)

      // draw
      const { w, h } = sizeRef.current
      ctx.clearRect(0, 0, w, h)

      // Lingering wisps finish their lives even after intensity drops to 0
      // (hover-out), so the smoke dissipates instead of vanishing mid-air.
      if (wispsRef.current.length === 0) {
        rafRef.current = requestAnimationFrame(loop)
        return
      }
      drawWisps(ctx, wispsRef.current, level)

      rafRef.current = requestAnimationFrame(loop)
    }

    rafRef.current = requestAnimationFrame(loop)

    return () => {
      cancelAnimationFrame(rafRef.current)
      ro.disconnect()
    }
  }, [windows, intensity, budget, pixelRatio, paused, level])

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

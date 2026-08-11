'use client'

/**
 * 设置页「炉火」tab 的 mini 实时预览
 *
 * 1:1 复用主调度器：调 fire-runtime（cavity/bed/glow/clip/stageGains/windowRect）
 * + smoke-runtime（spawn/update/draw/shouldSpawn/smokeBudget）的共享函数。
 *
 * 240x80 canvas,1 个 arch 窗口,永远 intensity=1。
 * 切换 effectId/smokeLevel 时清空粒子 + 重新 ramp。
 */

import { useEffect, useRef } from 'react'
import {
  EFFECTS,
  type FireEffectId,
  type Particle,
} from './fire-effects'
import {
  advanceIgnition,
  archPath,
  drawBedAndGlow,
  drawCavity,
  stageGains,
  windowRect,
} from './fire-runtime'
import {
  drawWisps,
  shouldSpawn,
  smokeBudget,
  spawnWisps,
  updateWisps,
  type Wisp,
} from './smoke-runtime'
import type { FurnaceWindow } from './bagua-furnace-fire'

// 画布比例 5:4（200x160），让单 arch 窗口有上下边距。
// PREVIEW_WIN 用 width:30% / height:60%，r=15% wh=60%，wh=4r → 直边清晰可见，
// 不再像横躺扁弧把整个画布塞满。
const PREVIEW_W = 200
const PREVIEW_H = 160

/** 1 个虚拟 arch 窗口（用 FurnaceWindow 协议），居中于 canvas */
const PREVIEW_WIN: FurnaceWindow = {
  id: 'preview',
  x: 50,
  width: 30,
  top: 20,
  height: 60,
  phase: 0,
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
  const wispsRef = useRef<Wisp[]>([])
  const progRef = useRef(0)
  const lastTsRef = useRef(0)

  // 切换 effect 时清空粒子 + 重新 ramp；smokeLevel 变化不重置已有烟雾
  useEffect(() => {
    particlesRef.current = []
    progRef.current = 0
    wispsRef.current = []
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
    const size = { w: PREVIEW_W, h: PREVIEW_H }

    const loop = (ts: number) => {
      const dt = Math.min(0.05, lastTsRef.current ? (ts - lastTsRef.current) / 1000 : 0.016)
      const time = ts / 1000
      lastTsRef.current = ts

      // 永远 ramp 到 1（与主场景 hover-in 一致）
      progRef.current = Math.min(1, progRef.current + dt / 0.6)
      const prog = progRef.current
      const gains = stageGains(prog)

      // 复用主调度器一样的 spawn/update 协议
      const effect = EFFECTS[effectId]
      const rect = windowRect(PREVIEW_WIN, size)
      const params = {
        win: PREVIEW_WIN,
        rect,
        dt,
        intensity: prog,
        gains,
        budget: { particles: 60, glow: true },
        pixelRatio: 1,
        time,
        seed: PREVIEW_WIN.phase,
      }
      effect.spawn(particlesRef.current, params)
      effect.update(particlesRef.current, dt, time, params)

      // 烟：与主场景同协议（shouldSpawn/spawnWisps/updateWisps/drawWisps）
      const budget = smokeBudget(smokeLevel)
      const smokeCount = shouldSpawn(prog, smokeLevel, wispsRef.current.length, budget, dt)
      if (smokeCount > 0) {
        spawnWisps(wispsRef.current, smokeCount, [PREVIEW_WIN], size)
      }
      updateWisps(wispsRef.current, dt)

      // 画：cavity + bed + glow + arch clip + 粒子 + 烟（全部走 runtime）
      ctx.clearRect(0, 0, PREVIEW_W, PREVIEW_H)
      ctx.save()
      ctx.beginPath()
      archPath(ctx, rect.wx, rect.wy, rect.ww, rect.wh)
      ctx.clip()
      drawCavity(ctx, [PREVIEW_WIN], size)
      drawBedAndGlow(ctx, [PREVIEW_WIN], size, gains, time)
      effect.draw(ctx, particlesRef.current, params)
      ctx.restore()
      drawWisps(ctx, wispsRef.current, smokeLevel)

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

/**
 * 炉火效果插件化接口（006-fire）
 *
 * 把「火焰怎么喷、怎么飞、怎么画」从调度器里抽出来，每个内置效果独立文件。
 * 调度器（bagua-furnace-fire.tsx）只负责：
 *  - 窗口裁剪 / 火口 cavity / staged ignition 推进
 *  - 把每个 dt 的 spawn/update/draw 转交给 effect
 *
 * 效果差异由 EFFECT 自身决定：粒子形状、调色板、混合模式、运动学。
 * 切换方式：改 `DEFAULT_EFFECT_ID` 即可；未来如需 URL 切换，只在 BaguaFurnace
 * 上加一个 prop 透传。
 */

import type { FurnaceWindow } from '../bagua-furnace-fire'

export type FireEffectId = 'plume' | 'ember' | 'flicker' | 'veil' | 'classic' | 'sanmei' | 'spark' | 'dragon'

export const DEFAULT_EFFECT_ID: FireEffectId = 'plume'

/** 窗口矩形缓存：调度器每帧为每个窗口算一次，传给 effect 避免重复 */
export interface WinRect {
  wx: number
  wy: number
  ww: number
  wh: number
}

/** 调度器每帧算好的「舞台增益」0..1，effect 可选用 */
export interface StageGains {
  warm: number // 火口暖色 wash
  bed: number // 床层呼吸
  glow: number // 窗口暖光
  ember: number // 火花上升
}

export interface FireParams {
  /** 该效果本帧要服务的窗口（调度器对每个窗口调用 spawn 一次） */
  win: FurnaceWindow
  rect: WinRect
  dt: number
  /** 全局点燃强度 0..1（已过 staged ignition） */
  intensity: number
  gains: StageGains
  budget: { particles: number; glow: boolean }
  pixelRatio: number
  /** 帧时间（秒），供正弦呼吸等用 */
  time: number
  /** 窗口序号 seed，避免三窗口同步呼吸 */
  seed: number
}

/** 最小粒子字段，effect 可在 update/draw 时自由扩展 */
export interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  life: number
  maxLife: number
  size: number
  seed: number
  /** effect 自由定义：颜色、形状、自定义运动参数 */
  [key: string]: number
}

export interface FireEffect {
  readonly id: FireEffectId
  /** 每个 useEffect 调一次（创建 ctx 引用 / 预算桶） */
  init?(ctx: EffectCtx): void
  /** 喷发：调度器为每个窗口调一次 */
  spawn(particles: Particle[], params: FireParams): void
  /** 推进：所有粒子运动学，effect 自行删 dead */
  update(particles: Particle[], dt: number, time: number, params: FireParams): void
  /** 绘制：ctx 已裁剪到窗口 + cavity 已铺底 */
  draw(ctx: CanvasRenderingContext2D, particles: Particle[], params: FireParams): void
}

/** effect 初始化时拿到的运行时引用，effect 不必自己抓 canvas/ctx */
export interface EffectCtx {
  pixelRatio: number
  budget: { particles: number; glow: boolean }
}

// registry：8 个内置 effect
import { plume } from './plume'
import { ember } from './ember'
import { flicker } from './flicker'
import { veil } from './veil'
import { classic } from './classic'
import { sanmei } from './sanmei'
import { spark } from './spark'
import { dragon } from './dragon'

export const EFFECTS: Record<FireEffectId, FireEffect> = {
  plume,
  ember,
  flicker,
  veil,
  classic,
  sanmei,
  spark,
  dragon,
}

export { plume, ember, flicker, veil, classic, sanmei, spark, dragon }

'use client'

import { useEffect, useRef, useState, useMemo, useSyncExternalStore } from 'react'
import Image from 'next/image'
import { BaguaFurnaceFire, FurnaceWindow } from './bagua-furnace-fire'
import { BaguaFurnaceSmoke } from './bagua-furnace-smoke'
import {
  DEFAULT_FIRE_EFFECT,
  DEFAULT_SMOKE_LEVEL,
  FIRE_EFFECT_CHANGE_EVENT,
  SMOKE_LEVEL_CHANGE_EVENT,
  FIRE_EFFECT_STORAGE_KEY,
  SMOKE_LEVEL_STORAGE_KEY,
  getFireEffect,
  getSmokeLevel,
} from '@/lib/fire-effect-pref'

interface BaguaFurnaceProps {
  alt: string
  windows: FurnaceWindow[]
  /** Override intensity (0-1). When set, hover/touch/prefers-reduced-motion is ignored —
   *  the caller owns fire behavior (e.g. 融合页常驻大火). */
  forceIntensity?: number
}

/**
 * Window cut-out mask as a data-URI SVG image.
 *
 * Chromium/WebKit do not resolve `mask-image: url(#id)` references to inline
 * SVG `<mask>` elements (Firefox does) — the failed reference masks the whole
 * element out, which made the cauldron invisible. An SVG *image* data URI
 * works everywhere. The mask applies in alpha mode, so the windows must be
 * genuinely transparent pixels: they are cut from one white path via
 * fill-rule="evenodd" (a black fill would stay opaque and cut nothing).
 */
function buildWindowCutoutMask(windows: FurnaceWindow[]): string {
  const holes = windows
    .map((win) => {
      const x = win.x - win.width / 2
      return archPath(x, win.top, win.width, win.height)
    })
    .join('')
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" preserveAspectRatio="none">` +
    `<path fill="white" fill-rule="evenodd" d="M0 0H100V100H0Z${holes}"/></svg>`
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`
}

/**
 * Arch window shape (0-100 viewBox units), matching the real holes in
 * /ding.png: semicircle top with radius = half the window width, straight
 * sides, straight bottom. Same shape as the fire canvas clip.
 */
function archPath(x: number, y: number, w: number, h: number): string {
  const r = w / 2
  return `M${x} ${y + r}A${r} ${r} 0 0 1 ${x + w} ${y + r}V${y + h}H${x}Z`
}

function useMediaQuery(query: string) {
  const [matches, setMatches] = useState(false)
  useEffect(() => {
    if (typeof window === 'undefined') return
    const m = window.matchMedia(query)
    const update = () => setMatches(m.matches)
    update()
    m.addEventListener?.('change', update)
    return () => m.removeEventListener?.('change', update)
  }, [query])
  return matches
}

/**
 * 火效/烟雾偏好存放在 localStorage（fire-effect-pref 托管），经
 * useSyncExternalStore 订阅：SSR/水合期用默认值，水合后自动重读；同标签页
 * CustomEvent + 跨标签页 storage 事件触发 onChange → React 重读快照。
 */
function subscribeFireEffect(onChange: () => void) {
  const onStorage = (e: StorageEvent) => {
    if (e.key === FIRE_EFFECT_STORAGE_KEY) onChange()
  }
  window.addEventListener(FIRE_EFFECT_CHANGE_EVENT, onChange)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(FIRE_EFFECT_CHANGE_EVENT, onChange)
    window.removeEventListener('storage', onStorage)
  }
}

function subscribeSmokeLevel(onChange: () => void) {
  const onStorage = (e: StorageEvent) => {
    if (e.key === SMOKE_LEVEL_STORAGE_KEY) onChange()
  }
  window.addEventListener(SMOKE_LEVEL_CHANGE_EVENT, onChange)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(SMOKE_LEVEL_CHANGE_EVENT, onChange)
    window.removeEventListener('storage', onStorage)
  }
}

const getDefaultFireEffect = () => DEFAULT_FIRE_EFFECT
const getDefaultSmokeLevel = () => DEFAULT_SMOKE_LEVEL

/** devicePixelRatio 只进 canvas effect（不进 DOM），lazy initializer 无水合风险 */
function getInitialPixelRatio() {
  if (typeof window === 'undefined') return 1
  return Math.min(2, window.devicePixelRatio || 1)
}

export function BaguaFurnace({ alt, windows, forceIntensity }: BaguaFurnaceProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const [isHovering, setIsHovering] = useState(false)
  const [isVisible, setIsVisible] = useState(true)
  const fireEffect = useSyncExternalStore(subscribeFireEffect, getFireEffect, getDefaultFireEffect)
  const smokeLevel = useSyncExternalStore(subscribeSmokeLevel, getSmokeLevel, getDefaultSmokeLevel)
  const isTouch = useMediaQuery('(hover: none), (pointer: coarse)')
  const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
  const [pixelRatio] = useState(getInitialPixelRatio)

  useEffect(() => {
    const el = rootRef.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      ([entry]) => {
        setIsVisible(entry?.isIntersecting ?? true)
      },
      { threshold: 0 }
    )
    io.observe(el)
    return () => io.disconnect()
  }, [])

  const budget = useMemo(() => {
    if (prefersReducedMotion) {
      return { fireParticles: 0, fireGlow: false, smokeWisps: 0 }
    }
    if (isTouch) {
      return { fireParticles: 140, fireGlow: false, smokeWisps: 35 }
    }
    return { fireParticles: 280, fireGlow: true, smokeWisps: 55 }
  }, [isTouch, prefersReducedMotion])

  const intensity = useMemo(() => {
    if (forceIntensity !== undefined) return forceIntensity
    if (prefersReducedMotion) return 0
    if (isTouch) return 1
    return isHovering ? 1 : 0
  }, [forceIntensity, isHovering, isTouch, prefersReducedMotion])

  const glowOpacity = useMemo(() => {
    if (isTouch || prefersReducedMotion) return 0
    return intensity
  }, [intensity, isTouch, prefersReducedMotion])

  // CSS glows join mid-ignition (delayed, slow fade-in); they leave quickly
  // on hover-out. Mirrors the staged canvas ignition in bagua-furnace-fire.
  const glowTransition =
    glowOpacity > 0 ? 'opacity 0.9s ease 0.3s' : 'opacity 0.7s ease 0s'

  const paused = !isVisible
  const cutoutMask = useMemo(() => buildWindowCutoutMask(windows), [windows])

  return (
    <div
      ref={rootRef}
      className="group/ding relative h-full w-full"
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => setIsHovering(false)}
    >
      {/* Fire lives behind the cauldron image so the bronze frame occludes it */}
      <BaguaFurnaceFire
        windows={windows}
        intensity={intensity}
        budget={{ particles: budget.fireParticles, glow: budget.fireGlow }}
        pixelRatio={pixelRatio}
        paused={paused}
        effectId={fireEffect}
      />

      {/* Cauldron image with window cut-outs */}
      <Image
        src="/ding.png"
        alt={alt}
        width={1024}
        height={1024}
        priority
        className="h-full w-full"
        style={{
          WebkitMaskImage: cutoutMask,
          maskImage: cutoutMask,
          WebkitMaskSize: '100% 100%',
          maskSize: '100% 100%',
          WebkitMaskRepeat: 'no-repeat',
          maskRepeat: 'no-repeat',
        }}
      />

      {/*
        Unmodified cauldron image covering the cut-out layer while unlit, so
        the banner at rest shows /ding.png exactly as-is (original black arch
        windows, no cavity/approximation). Fades out slowly when the fire is
        lit, cross-fading the windows into the warming cavity — no shape pop,
        because the cut-out outline differs from the original arches.
      */}
      <Image
        src="/ding.png"
        alt=""
        aria-hidden
        width={1024}
        height={1024}
        priority
        className="pointer-events-none absolute inset-0 h-full w-full"
        style={{ opacity: intensity > 0 ? 0 : 1, transition: 'opacity 0.6s ease' }}
      />

      {/* Warm rim glow behind the windows — revealed with the fire (desktop only) */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          opacity: glowOpacity,
          transition: glowTransition,
          background:
            'radial-gradient(ellipse 70% 28% at 50% 56%, rgba(255,140,50,0.12) 0%, transparent 60%)',
          mixBlendMode: 'screen',
        }}
      />

      {/* Warm body glow around the furnace windows — revealed with the fire (desktop only) */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          opacity: glowOpacity,
          transition: glowTransition,
          background:
            'radial-gradient(ellipse 60% 18% at 50% 55%, rgba(255,120,30,0.14) 0%, rgba(220,70,15,0.06) 40%, transparent 68%)',
          mixBlendMode: 'screen',
        }}
      />

      {/* Smoke rises in front of the furnace */}
      <BaguaFurnaceSmoke
        windows={windows}
        intensity={intensity}
        budget={{ wisps: budget.smokeWisps }}
        pixelRatio={pixelRatio}
        paused={paused}
        level={smokeLevel}
      />
    </div>
  )
}

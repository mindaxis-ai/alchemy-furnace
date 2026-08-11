'use client'

/**
 * 设置页「炉火」tab 内容
 *
 * 顶部:实时 mini 预览(随 effect/smoke 变化)
 * 中部:4 个火效选项卡(高亮选中)
 * 底部:烟雾浓度滑块 0..1 (0% = 无烟, 100% = 浓烟)
 *
 * 选择立即写 localStorage + 派 CustomEvent,
 * 跨标签页同步走 storage event(均由 pref 模块托管)。
 */

import { useEffect, useState } from 'react'
import { useTranslations } from 'next-intl'
import { Flame } from 'lucide-react'
import { cn } from '@/lib/utils'
import { FireEffectPreview } from '@/components/alchemy/fire-effect-preview'
import {
  FIRE_EFFECT_OPTIONS,
  FIRE_EFFECT_CHANGE_EVENT,
  SMOKE_LEVEL_CHANGE_EVENT,
  type FireEffectId,
  getFireEffect,
  getSmokeLevel,
  setFireEffect,
  setSmokeLevel,
} from '@/lib/fire-effect-pref'

export function FireEffectPanel() {
  const t = useTranslations('settings.fire')
  const [selected, setSelected] = useState<FireEffectId>('plume')
  const [level, setLevel] = useState<number>(0.55)

  // mount: 从 localStorage 读
  useEffect(() => {
    setSelected(getFireEffect())
    setLevel(getSmokeLevel())
  }, [])

  // 监听同标签页 / 跨标签页变化 → 同步到 UI（用户在另一处改了也能反映）
  useEffect(() => {
    const onFire = (e: Event) => setSelected((e as CustomEvent).detail)
    const onSmoke = (e: Event) => setLevel((e as CustomEvent).detail)
    const onStorage = (e: StorageEvent) => {
      if (e.key === 'alchemy.fireEffect') setSelected(getFireEffect())
      if (e.key === 'alchemy.smokeLevel') setLevel(getSmokeLevel())
    }
    window.addEventListener(FIRE_EFFECT_CHANGE_EVENT, onFire)
    window.addEventListener(SMOKE_LEVEL_CHANGE_EVENT, onSmoke)
    window.addEventListener('storage', onStorage)
    return () => {
      window.removeEventListener(FIRE_EFFECT_CHANGE_EVENT, onFire)
      window.removeEventListener(SMOKE_LEVEL_CHANGE_EVENT, onSmoke)
      window.removeEventListener('storage', onStorage)
    }
  }, [])

  const pickEffect = (id: FireEffectId) => {
    setSelected(id)
    setFireEffect(id)
  }
  const pickLevel = (n: number) => {
    setLevel(n)
    setSmokeLevel(n)
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 标准页头（与 ModelsPanel/AboutPanel 同款版式） */}
      <div className="flex items-center gap-3 mb-6 min-w-0">
        <Flame className="w-6 h-6 text-gold shrink-0" />
        <div className="min-w-0">
          <h1 className="page-title truncate">{t('title')}</h1>
          <p className="page-subtitle truncate">{t('subtitle')}</p>
        </div>
      </div>

      <div className="space-y-6">
        {/* 预览 */}
        <section className="dao-card p-6">
          <h2 className="text-base font-serif font-bold text-gold mb-4">
            {t('preview')}
          </h2>
          <FireEffectPreview effectId={selected} smokeLevel={level} />
        </section>

        {/* 4 个火效选项卡 */}
        <section className="dao-card p-5">
          <h2 className="text-base font-serif font-bold text-gold mb-4">
            {t('effectLabel')}
          </h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {FIRE_EFFECT_OPTIONS.map((id) => {
              const isOn = id === selected
              return (
                <button
                  key={id}
                  type="button"
                  onClick={() => pickEffect(id)}
                  aria-pressed={isOn}
                  className={cn(
                    'flex flex-col items-start gap-1.5 rounded-2xl border p-3.5 text-left transition-all duration-200 min-w-0',
                    isOn
                      ? 'border-gold/60 bg-gold/5 ring-1 ring-gold/40 shadow-[0_15px_30px_-12px_rgba(201,169,110,0.35)]'
                      : 'border-border/70 bg-card/60 hover:border-gold/30 hover:bg-gold/5'
                  )}
                >
                  <Flame
                    className={cn(
                      'size-4 shrink-0',
                      isOn ? 'text-primary' : 'text-sage'
                    )}
                  />
                  <span className="font-serif text-sm font-bold text-foreground truncate w-full">
                    {t(`options.${id}.label`)}
                  </span>
                  <span className="text-[11px] leading-snug text-sage line-clamp-2">
                    {t(`options.${id}.desc`)}
                  </span>
                </button>
              )
            })}
          </div>
        </section>

        {/* 烟雾浓度滑块 */}
        <section className="dao-card p-5">
          <div className="flex items-baseline justify-between mb-2 gap-2 min-w-0">
            <h2 className="text-base font-serif font-bold text-gold truncate">
              {t('smokeLabel')}
            </h2>
            <span className="text-sm font-mono text-sage shrink-0 whitespace-nowrap">
              {Math.round(level * 100)}%
            </span>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            step={1}
            value={Math.round(level * 100)}
            onChange={(e) => pickLevel(Number(e.target.value) / 100)}
            aria-label={t('smokeLabel')}
            className="alchemy-range w-full"
          />
          <p className="mt-2 text-[11px] text-sage">{t('smokeHint')}</p>
        </section>
      </div>
    </div>
  )
}

'use client'

import { useEffect, useState } from 'react'
import Image from 'next/image'
import Link from 'next/link'
import { ChevronRight, Flame, Plus } from 'lucide-react'
import { useLocale, useTranslations } from 'next-intl'
import { useAgent } from '@/contexts/AgentContext'
import { useChat } from '@/contexts/ChatContext'
import { pillItemDetailHref, recipeDetailHref } from '@/lib/entity-detail-route'
import { getPillItem, listPillItems } from '@/services/pillInventoryService'
import { listRecipes } from '@/services/recipeService'
import type { PillItemDetail, PillItemListItem, RecipeListItem } from '@/services/types'
import { RecentSessionList } from '@/components/home/recent-session-list'
import { FloatCard, CoinIcon } from '@/components/alchemy/float-card'
import { BaguaFurnace } from '@/components/alchemy/bagua-furnace'
import type { FurnaceWindow } from '@/components/alchemy/bagua-furnace-fire'
import { formatDateTime } from '@/utils/format'

// Image-relative furnace-window geometry (percent of /ding.png), measured
// from the PNG's near-black connected components: arch windows — semicircle
// top (radius = width/2), straight sides and bottom.
const FURNACE_WINDOWS: FurnaceWindow[] = [
  { id: 'left',   x: 37.35, width: 7.71, top: 50.10, height: 9.08, phase: 0.00 },
  { id: 'center', x: 50.29, width: 8.79, top: 50.49, height: 8.98, phase: 0.45 },
  { id: 'right',  x: 63.18, width: 7.81, top: 50.20, height: 9.08, phase: 0.90 },
]

/**
 * Home page (the furnace / 鼎 hero).
 *
 * All user-visible strings come from the i18n message dictionary
 * (`messages/<locale>.json` → `home.*`). The flame sub-rendering inside
 * `<DingHero />` is intentionally untouched — its coordinates and
 * keyframes are not translated. `setRequestLocale` for static
 * rendering is invoked in `app/[locale]/layout.tsx`.
 */
export default function HomePage() {
  const t = useTranslations('home')
  const tStats = useTranslations('home.stats')
  const tSpot = useTranslations('home.spotlight')
  const tRecipes = useTranslations('home.recipeList')
  const tSessions = useTranslations('home.sessions')
  const tCloser = useTranslations('home.closer')
  const tHero = useTranslations('home.hero')

  // 标题字号按语言分档：中文 3 字用 17rem 还原原始视觉，英文 15 字降到 8.5rem 防越界
  const locale = useLocale()
  const isZh = locale === 'zh-CN'
  const titleSize = isZh
    ? 'text-[16vw] sm:text-[10rem] md:text-[14rem] lg:text-[17rem]'
    : 'text-[16vw] sm:text-[8rem] md:text-[7rem] lg:text-[8.5rem]'
  // max-w 也要按语言分：中文 2 字一行需要更宽容器，英文则继续约束防越界
  const titleMaxW = isZh
    ? 'md:max-w-[60%] lg:max-w-[55%]'
    : 'md:max-w-[42%] lg:max-w-[38%]'

  // 金丹消耗品语义:库存 = 可用实例(available,服用/融合即消耗),丹方 = 永久保留的配方
  const [inventory, setInventory] = useState<{ total: number; items: PillItemListItem[] }>({
    total: 0,
    items: [],
  })
  const [recipes, setRecipes] = useState<{ total: number; items: RecipeListItem[] }>({
    total: 0,
    items: [],
  })
  const [spotlightDetail, setSpotlightDetail] = useState<PillItemDetail | null>(null)
  const { state: agentState, fetchAgents } = useAgent()
  const { state: chatState, fetchSessions } = useChat()

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const [inv, recs] = await Promise.all([
          listPillItems({ size: 4 }),
          listRecipes({ size: 4 }),
        ])
        if (cancelled) return
        setInventory(inv)
        setRecipes(recs)
        // spotlight 详情富化(描述/标签);失败时以列表字段兜底,不打断首页
        const top = inv.items[0]
        if (top) {
          try {
            const detail = await getPillItem(top.id)
            if (!cancelled) setSpotlightDetail(detail)
          } catch {
            // 兜底展示列表字段
          }
        }
      } catch {
        // 库存/丹方拉取失败:保持空态
      }
    })()
    fetchAgents()
    fetchSessions()
    return () => {
      cancelled = true
    }
  }, [fetchAgents, fetchSessions])

  const agents = agentState.agents
  const sessions = chatState.sessions

  const stats = [
    {
      key: 'inventory' as const,
      label: tStats('inventory.label'),
      value: inventory.total,
      unit: tStats('inventory.unit'),
      caption: tStats('inventory.caption'),
    },
    {
      key: 'agents' as const,
      label: tStats('agents.label'),
      value: agents.length,
      unit: tStats('agents.unit'),
      caption: tStats('agents.caption'),
    },
    {
      key: 'sessions' as const,
      label: tStats('sessions.label'),
      value: sessions.length,
      unit: tStats('sessions.unit'),
      caption: tStats('sessions.caption'),
    },
    {
      key: 'recipes' as const,
      label: tStats('recipes.label'),
      value: recipes.total,
      unit: tStats('recipes.unit'),
      caption: tStats('recipes.caption'),
    },
  ]

  // 最新库存实例 + 详情富化;无库存时走空态引导去丹方炼制
  const spotlightItem = inventory.items[0] ?? null
  const spotlight = spotlightItem
    ? {
        ...spotlightItem,
        description: spotlightDetail?.description ?? '',
        tags: spotlightDetail?.tags ?? [],
      }
    : null
  const recentRecipes = recipes.items

  return (
    <div className="pb-24">
      {/* ── hero：通栏铺满，标题压着鼎交叠 ── */}
      <header
        id="hero"
        className="relative isolate overflow-hidden px-5 pt-8 sm:px-8 md:flex md:min-h-[calc(100vh-4rem)] md:flex-col md:justify-center md:pt-0 lg:px-14"
      >
        {/* 鼎：更大更靠左,让标题尾部压在其上 */}
        {/* 鼎：参考 / 炉子动画 的 float-slow + 径向 mask;hover 点火冒烟 */}
        {/* 外层 absolute 负责定位;内层 group/ding + relative 是火/烟 absolute 子元素的包含块 */}
        <div className="relative mx-auto my-6 w-[60%] opacity-90 md:absolute md:right-[6%] md:top-1/2 md:my-0 md:w-[72%] md:-translate-y-1/2 lg:w-[64%]">
          <div className="group/ding relative w-full">
            {/* 鼎体：scale(1.15) 放大（hero 比例已通过 Image 高度约束协调） */}
            <div className="origin-center" style={{ transform: 'scale(1.15)' }}>
              <div
                className="float-slow relative"
                style={{
                  WebkitMaskImage:
                    'radial-gradient(130% 120% at 68% 42%, #000 58%, transparent 90%)',
                  maskImage:
                    'radial-gradient(130% 120% at 68% 42%, #000 58%, transparent 90%)',
                }}
              >
                <div
                  className="relative ml-auto mr-0"
                  style={{
                    height: 'calc(100vh - 4rem)',
                    aspectRatio: '1 / 1',
                    maxWidth: '100%',
                  }}
                >
                <BaguaFurnace alt={t('dingAlt')} windows={FURNACE_WINDOWS} />
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className={`relative z-10 mx-auto max-w-3xl text-center md:mx-0 md:-mt-[14vh] md:text-left ${titleMaxW}`}>
          <h1 className="font-serif font-black leading-[1.06] tracking-tight text-foreground">
            <span className={`block whitespace-nowrap ${titleSize}`}>
              {tHero('titlePart1')}
            </span>
            <span className={`block whitespace-nowrap pl-[0.08em] text-primary ${titleSize}`}>
              {tHero('titlePart2')}
            </span>
          </h1>
        </div>

        {/* 底部横排题字：桌面端钉在 hero 左下角,作画轴落款 */}
        <p className="relative z-10 mt-14 flex max-w-full items-center gap-4 truncate font-serif text-xl font-bold tracking-[0.2em] md:absolute md:bottom-10 md:left-8 md:mt-0 md:max-w-[calc(100%-4rem)] md:tracking-[0.5em] lg:left-14 lg:max-w-[40%]">
          <span className="h-px w-10 shrink-0 bg-gold/70" aria-hidden />
          <span className="truncate">{tHero('banner')}</span>
        </p>
      </header>

      <main className="px-5 sm:px-8 lg:px-14">
        {/* ── 炉房概览：真实数据账簿 ── */}
        <section
          id="stats"
          aria-label={tStats('ariaLabel')}
          className="mt-8 grid grid-cols-2 overflow-hidden rounded-[20px] border border-border/70 bg-card/50 backdrop-blur-sm md:grid-cols-4"
        >
          {stats.map((s, i) => (
            <div
              key={s.key}
              className={[
                'group relative p-7 transition-colors duration-500 hover:bg-card',
                i % 2 === 0 ? 'border-r border-border/70' : '',
                i < 2 ? 'border-b border-border/70 md:border-b-0' : '',
                i === 2 ? 'md:border-r' : '',
                i === 1 ? 'md:border-r' : '',
              ].join(' ')}
            >
              <span className="flex items-center gap-2 text-xs tracking-widest text-sage">
                <span className="size-1 rounded-full bg-primary/70" aria-hidden />
                {s.label}
              </span>
              <div className="mt-6 flex items-baseline gap-1">
                <span className="font-serif text-5xl font-black leading-none tracking-tight text-foreground">
                  {s.value}
                </span>
                {s.unit && (
                  <span className="text-base text-muted-foreground">{s.unit}</span>
                )}
              </div>
              <p className="mt-3 text-sm leading-relaxed text-muted-foreground">{s.caption}</p>
              <span
                aria-hidden
                className="absolute inset-x-7 bottom-0 h-px origin-left scale-x-0 bg-primary/60 transition-transform duration-500 group-hover:scale-x-100"
              />
            </div>
          ))}
        </section>

        {/* ── 金丹阁：最新库存实例 + 丹方录 ── */}
        <section id="pills" className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)]">
          {/* 最新金丹 spotlight */}
          <div className="relative flex h-full flex-col overflow-hidden rounded-[20px] border border-border/70 bg-card/60 shadow-[0_25px_50px_-12px_rgba(60,40,20,0.08)]">
            <div className="relative flex h-full flex-col gap-8 p-8 md:p-10">
              <div className="flex items-center justify-between">
                <span className="flex items-center gap-1.5 rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
                  <Flame className="size-3.5" strokeWidth={2} aria-hidden />
                  {tSpot('badge')}
                </span>
                <Link href="/pills" className="font-mono text-xs text-muted-foreground transition-colors hover:text-primary">
                  {tSpot('viewAll')}
                </Link>
              </div>

              {spotlight ? (
                <div className="max-w-md">
                  <p className="text-sm tracking-widest text-sage">
                    {tSpot('version', { revision: spotlight.revision })} · {formatDateTime(spotlight.created_at)}
                  </p>
                  <h2 className="mt-2 text-balance font-serif text-5xl font-black leading-[0.95] text-foreground md:text-6xl">
                    {spotlight.name}
                  </h2>
                  <p className="mt-5 line-clamp-3 text-pretty leading-relaxed text-muted-foreground">
                    {spotlight.description || tSpot('noDescription')}
                  </p>
                  {spotlight.tags.length > 0 && (
                    <div className="mt-5 flex flex-wrap gap-2">
                      {spotlight.tags.map((tag) => (
                        <span key={tag} className="rounded-full bg-secondary px-2.5 py-0.5 text-xs text-sage">
                          {tag}
                        </span>
                      ))}
                    </div>
                  )}
                  <Link
                    href={pillItemDetailHref(spotlight.id)}
                    className="mt-8 inline-flex w-fit items-center gap-2 rounded-full bg-foreground px-7 py-3 text-sm font-medium text-background transition-transform duration-300 hover:scale-[1.03]"
                  >
                    {tSpot('viewCta')}
                  </Link>
                </div>
              ) : (
                <div className="flex flex-1 flex-col items-start justify-center gap-4">
                  <p className="font-serif text-3xl font-black text-foreground">
                    {tSpot('emptyTitle')}
                  </p>
                  <p className="max-w-sm leading-relaxed text-muted-foreground">
                    {tSpot('emptyDesc')}
                  </p>
                  <Link
                    href="/recipes"
                    className="inline-flex items-center gap-2 rounded-full bg-primary px-7 py-3 text-sm font-medium text-primary-foreground transition-transform duration-300 hover:scale-[1.03]"
                  >
                    <Plus className="size-4" aria-hidden />
                    {tSpot('emptyCta')}
                  </Link>
                </div>
              )}
            </div>
          </div>

          {/* 丹方录：最新四张丹方（永久保留；显示版本与可用库存数） */}
          <FloatCard delay={0.2} className="h-full">
            <div className="flex h-full flex-col gap-6 p-7">
              <div className="flex items-baseline justify-between">
                <h3 className="font-serif text-xl font-black text-foreground">
                  {tRecipes('title')}
                </h3>
                <span className="text-xs text-sage">
                  {tRecipes('count', { count: recipes.total })}
                </span>
              </div>

              <ul className="flex flex-col gap-2">
                {recentRecipes.map((r, i) => (
                  <li key={r.id}>
                    <Link
                      href={recipeDetailHref(r.id)}
                      className="group flex w-full items-center gap-4 rounded-2xl px-2 py-2.5 text-left transition-colors duration-300 hover:bg-secondary/70"
                    >
                      <CoinIcon tone={(['gold', 'sage', 'cinnabar'] as const)[i % 3]}>
                        <span className="font-serif text-base font-bold">{r.name.slice(0, 1)}</span>
                      </CoinIcon>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <p className="truncate font-serif text-base font-bold text-foreground">
                            {r.name}
                          </p>
                          <span className="shrink-0 rounded-full bg-accent px-2 py-0.5 text-[11px] font-medium text-accent-foreground">
                            {tRecipes('versionBadge', { revision: r.revision })}
                          </span>
                        </div>
                        <p className="mt-1 truncate text-sm text-muted-foreground">
                          {tRecipes('availableCount', { count: r.available_count })}
                        </p>
                      </div>
                      <ChevronRight
                        className="size-4 shrink-0 text-muted-foreground transition-transform duration-300 group-hover:translate-x-0.5"
                        strokeWidth={1.75}
                        aria-hidden
                      />
                    </Link>
                  </li>
                ))}
                {recentRecipes.length === 0 && (
                  <li className="py-6 text-center text-sm text-muted-foreground">
                    {tRecipes('empty')}
                  </li>
                )}
              </ul>
            </div>
          </FloatCard>
        </section>

        {/* ── 论道旧录 + 收尾 ── */}
        <section id="log" className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.7fr)]">
          <FloatCard delay={0.4} className="h-full">
            <div className="flex h-full flex-col gap-6 p-7">
              <div className="flex items-baseline justify-between">
                <h3 className="font-serif text-xl font-black text-foreground">
                  {tSessions('title')}
                </h3>
                <span className="text-xs text-sage">
                  {tSessions('count', { count: sessions.length })}
                </span>
              </div>

              <RecentSessionList sessions={sessions} />
            </div>
          </FloatCard>

          <div className="relative flex h-full flex-col justify-between overflow-hidden rounded-[20px] border border-border/70 bg-card/60 p-10 shadow-[0_25px_50px_-12px_rgba(60,40,20,0.08)]">
            <div
              aria-hidden
              className="ink-fade pointer-events-none absolute -right-10 -top-10 size-52 rounded-full opacity-60"
              style={{
                background: 'radial-gradient(circle, rgba(201,169,110,0.16), transparent 65%)',
              }}
            />
            <div className="relative flex items-start justify-between gap-6">
              <div>
                <p className="font-serif text-3xl font-black text-foreground">
                  {tCloser('title')}
                </p>
                <p className="mt-4 max-w-sm text-pretty leading-relaxed text-muted-foreground">
                  {tCloser('body')}
                </p>
              </div>
              <div className="grid size-20 shrink-0 place-items-center rounded-2xl bg-primary text-primary-foreground shadow-[0_20px_40px_-12px_rgba(181,74,63,0.45)]">
                <span className="font-serif text-3xl font-black leading-none">
                  {tCloser('stamp')}
                </span>
              </div>
            </div>
            <Link
              href="/chat"
              className="relative mt-10 inline-flex w-fit items-center gap-2 rounded-full bg-foreground px-7 py-3 text-sm font-medium text-background transition-transform duration-300 hover:scale-[1.03]"
            >
              {tCloser('cta')}
            </Link>
          </div>
        </section>
      </main>
    </div>
  )
}

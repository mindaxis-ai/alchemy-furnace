'use client'

/**
 * 设置页 tab 容器（client）
 * ?tab=models|about，默认 models；非法值回退 models
 * tab 切换用 router.replace 同步 URL（不产生 history 记录、不滚动）
 */
import { useRouter, useSearchParams } from 'next/navigation'
import { useTranslations } from 'next-intl'
import { Info, Flame, ExternalLink, Heart } from 'lucide-react'
import { TopTabs } from '@/components/interaction/top-tabs'
import { ModelsPanel } from '@/components/models/models-panel'
import { FireEffectPanel } from '@/components/settings/fire-effect-panel'

const TAB_KEYS = ['models', 'fire', 'about'] as const
type TabKey = (typeof TAB_KEYS)[number]

function isTabKey(v: string | null): v is TabKey {
  return v === 'models' || v === 'fire' || v === 'about'
}

export function SettingsTabs() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const t = useTranslations('settings.tabs')
  const tAbout = useTranslations('about')

  const raw = searchParams.get('tab')
  const active: TabKey = isTabKey(raw) ? raw : 'models'

  const switchTab = (key: string) => {
    router.replace(key === 'models' ? '/settings' : `/settings?tab=${key}`, {
      scroll: false,
    })
  }

  return (
    <>
      <div className="mx-auto max-w-6xl px-4 pt-6 sm:px-6">
        <TopTabs
          tabs={[
            { key: 'models', label: t('models') },
            { key: 'fire', label: t('fire') },
            { key: 'about', label: t('about') },
          ]}
          activeKey={active}
          onChange={switchTab}
        />
      </div>
      {active === 'models' ? (
        <ModelsPanel />
      ) : active === 'fire' ? (
        <FireEffectPanel />
      ) : (
        <AboutPanel />
      )}
    </>
  )
}

/** 关于区：从旧 settings/page.tsx 平移（关于炼丹炉 + 技术栈卡片），补标准页头与 models tab 同宽 */
function AboutPanel() {
  const tAbout = useTranslations('about')
  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部（与道人府/ModelsPanel 同款版式） */}
      <div className="flex items-center gap-3 mb-6 min-w-0">
        <Info className="w-6 h-6 text-gold shrink-0" />
        <div className="min-w-0">
          <h1 className="page-title truncate">{tAbout('pageTitle')}</h1>
          <p className="page-subtitle truncate">{tAbout('pageSubtitle')}</p>
        </div>
      </div>

      <div className="space-y-6">
        <section className="dao-card p-5">
          <div className="flex items-center gap-2 mb-4 min-w-0">
            <Info className="w-5 h-5 text-gold shrink-0" />
            <h2 className="text-base font-serif font-bold text-gold truncate">{tAbout('sectionTitle')}</h2>
          </div>

          <div className="flex flex-col items-center text-center py-4">
            <div className="w-16 h-16 rounded-2xl bg-gradient-to-b from-primary/15 to-muted border-2 border-gold/30 flex items-center justify-center mb-3 shadow-[0_15px_30px_-12px_rgba(201,169,110,0.5)]">
              <Flame className="w-8 h-8 text-primary" />
            </div>
            <h3 className="text-lg font-serif font-bold text-gold mb-1">{tAbout('productName')}</h3>
            <p className="text-xs text-muted-foreground mb-4">v1.0.0</p>

            <p className="text-sm text-muted-foreground leading-relaxed mb-4 max-w-md">
              {tAbout('productBody1')}
              {tAbout('productBody2')}
            </p>

            <div className="dao-divider text-[10px] w-full mb-4">
              <Heart className="w-3 h-3" />
            </div>

            <div className="space-y-2 w-full text-left">
              <a
                href="https://github.com/yusanwen-code/alchemy-furnace"
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-2 p-2.5 rounded-lg bg-muted hover:bg-gold/5 border border-border/70 hover:border-gold/40 transition-all text-sm min-w-0"
              >
                <ExternalLink className="w-4 h-4 text-gold shrink-0" />
                <span className="text-foreground truncate">{tAbout('githubRepo')}</span>
              </a>
            </div>
          </div>
        </section>

        <section className="dao-card p-5">
          <h3 className="text-sm font-medium text-gold mb-3">{tAbout('techStack')}</h3>
          <div className="flex flex-wrap gap-2">
            {tAbout.raw('techList').map((tech: string) => (
              <span
                key={tech}
                className="px-2.5 py-1 text-[11px] rounded-full bg-sage/10 text-sage border border-sage/20 whitespace-nowrap"
              >
                {tech}
              </span>
            ))}
          </div>
        </section>
      </div>
    </div>
  )
}

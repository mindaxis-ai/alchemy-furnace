import { useTranslations } from 'next-intl'
import { SealDot } from '@/components/alchemy/float-card'

/** 宣纸风页脚：细边框、衬线落款、印章装饰 */
export function Footer() {
  const t = useTranslations('footer')
  return (
    <footer className="border-t border-border/70">
      <div className="mx-auto flex max-w-6xl flex-col items-center gap-4 px-4 py-10 sm:flex-row sm:justify-between sm:px-6">
        <p className="font-serif text-sm font-bold text-foreground min-w-0 text-center sm:text-left">
          <span className="truncate">{t('brand')}</span>
          <span className="mx-1 text-border">·</span>
          <span className="font-normal text-muted-foreground">{t('tagline')}</span>
        </p>
        <div className="flex items-center gap-2 text-xs text-sage shrink-0">
          <SealDot />
          <span className="font-mono tracking-wide whitespace-nowrap">Skill-Persona Alchemy</span>
        </div>
      </div>
    </footer>
  )
}

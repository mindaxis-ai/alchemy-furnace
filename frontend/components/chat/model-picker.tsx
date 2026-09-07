'use client'

import { useEffect, useState } from 'react'
import { useTranslations } from 'next-intl'
import { ChevronDown, RotateCw } from 'lucide-react'
import { options, type ModelOption } from '@/services/modelService'

/** A request-scoped model choice; agent defaults remain untouched. */
export function ModelPicker({ value, onChange, disabled }: {
  value: string
  onChange: (value: string) => void
  disabled: boolean
}) {
  const t = useTranslations('chatView.model')
  const [models, setModels] = useState<ModelOption[]>([])
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let active = true
    void options().then(result => {
      if (active) { setModels(result); setStatus('ready') }
    }).catch(() => { if (active) setStatus('error') })
    return () => { active = false }
  }, [attempt])

  if (status === 'error') {
    return <button type="button" disabled={disabled} aria-label={t('retry')} title={t('loadError')}
      onClick={() => { setStatus('loading'); setAttempt(value => value + 1) }}
      className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs text-muted-foreground hover:bg-secondary disabled:opacity-50">
      <RotateCw className="size-3.5" />{t('retry')}
    </button>
  }

  return <div className="relative min-w-0 max-w-[min(65vw,280px)]">
    <select aria-label={t('label')} title={t('hint')} value={value}
      disabled={disabled || status === 'loading'} onChange={event => onChange(event.target.value)}
      className="w-full cursor-pointer appearance-none truncate rounded-lg bg-transparent py-1.5 pl-2 pr-7 text-xs text-foreground outline-none hover:bg-secondary focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50">
      <option value="">{t(status === 'loading' ? 'loading' : 'agentDefault')}</option>
      {value && !models.some(model => model.name === value) && <option value={value}>{value}</option>}
      {models.map(model => <option key={`${model.provider_name}/${model.name}`} value={model.name}>
        {model.display_name || model.name} · {model.provider_display_name || model.provider_name}
      </option>)}
    </select>
    <ChevronDown aria-hidden className="pointer-events-none absolute right-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
  </div>
}

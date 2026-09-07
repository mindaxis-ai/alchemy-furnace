'use client'

import { useEffect, useState, useSyncExternalStore } from 'react'
import { Activity, Bug, Check, Clipboard, FileText, Loader2, RefreshCw, X } from 'lucide-react'
import { useTranslations } from 'next-intl'
import { isDesktop } from '@/services/api'
import { getDiagnostics, type DesktopDiagnostics } from '@/services/diagnosticsService'
import { listApiFailures, type RecentApiFailure } from '@/lib/diagnostics/recent-api-failures'
import {
  getDefaultPromptDebugEnabled,
  getPromptDebugEnabled,
  setPromptDebugEnabled,
  subscribePromptDebug,
} from '@/lib/prompt-debug-pref'

export function DiagnosticsPanel() {
  const t = useTranslations('settings.diagnostics')
  const [data, setData] = useState<DesktopDiagnostics | null>(null)
  const [failures, setFailures] = useState<RecentApiFailure[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const promptDebug = useSyncExternalStore(
    subscribePromptDebug,
    getPromptDebugEnabled,
    getDefaultPromptDebugEnabled,
  )

  const refresh = () => {
    setLoading(true)
    setError(null)
    setFailures(listApiFailures())
    getDiagnostics().then(setData).catch((e: unknown) => {
      setError(e instanceof Error ? e.message : t('loadFailed'))
    }).finally(() => setLoading(false))
  }

  useEffect(() => {
    const timer = window.setTimeout(refresh, 0)
    return () => window.clearTimeout(timer)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  if (!isDesktop()) return null

  const copySummary = async () => {
    const summary = [
      `${t('title')}`,
      `${t('python')}: ${data?.python_engine ?? t('unknown')}`,
      `${t('logDir')}: ${data?.log_dir ?? '-'}`,
      ...failures.slice(0, 10).map(f => `${f.at} ${f.method} ${f.path} ${f.status} ${f.errorCode ?? '-'} ${f.requestId ?? '-'}`),
    ].join('\n')
    await navigator.clipboard.writeText(summary)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1600)
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <div className="mb-6 flex items-start gap-3">
        <Activity className="mt-1 h-6 w-6 shrink-0 text-gold" />
        <div className="min-w-0"><h1 className="page-title">{t('title')}</h1><p className="page-subtitle">{t('subtitle')}</p></div>
      </div>
      {loading && !data ? <div className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{t('loading')}</div> : (
        <div className="space-y-6">
          <section className="dao-card p-5">
            <div className="flex items-center justify-between gap-4">
              <div className="flex min-w-0 items-start gap-3">
                <Bug className="mt-0.5 h-4 w-4 shrink-0 text-gold" />
                <div className="min-w-0">
                  <h2 className="font-serif text-base font-bold text-gold">{t('promptDebugTitle')}</h2>
                  <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t('promptDebugDescription')}</p>
                </div>
              </div>
              <button
                type="button"
                role="switch"
                aria-label={t('promptDebugLabel')}
                aria-checked={promptDebug}
                onClick={() => setPromptDebugEnabled(!promptDebug)}
                className={`relative h-6 w-11 shrink-0 overflow-hidden rounded-full border p-0 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60 ${promptDebug ? 'border-gold/70 bg-gold/70' : 'border-border bg-muted'}`}
              >
                <span className={`absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-background shadow-sm transition-transform ${promptDebug ? 'translate-x-5' : 'translate-x-0'}`} />
              </button>
            </div>
          </section>
          <section className="dao-card p-5">
            <div className="mb-4 flex items-center justify-between gap-3"><h2 className="font-serif text-base font-bold text-gold">{t('serviceStatus')}</h2><span className="inline-flex items-center gap-1 text-xs text-sage">{data?.python_engine === 'ok' ? <Check className="h-3.5 w-3.5" /> : <X className="h-3.5 w-3.5 text-primary" />}{data?.python_engine === 'ok' ? t('running') : t('unavailable')}</span></div>
            {error && <p role="alert" className="mb-3 text-sm text-primary">{error}</p>}
            <div className="space-y-2 text-xs text-muted-foreground"><p>{t('logDir')}: <code className="break-all text-foreground">{data?.log_dir ?? '-'}</code></p><p>{t('appLog')}: <code className="break-all text-foreground">{data?.app_log ?? '-'}</code></p><p>{t('pythonLog')}: <code className="break-all text-foreground">{data?.python_log ?? '-'}</code></p></div>
            <div className="mt-4 flex flex-wrap gap-2"><button type="button" onClick={refresh} className="dao-btn-ghost"><RefreshCw className="h-4 w-4" />{t('refresh')}</button><button type="button" onClick={copySummary} className="dao-btn-primary">{copied ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}{copied ? t('copied') : t('copy')}</button></div>
          </section>
          <section className="dao-card p-5"><div className="mb-4 flex items-center gap-2"><FileText className="h-4 w-4 text-gold" /><h2 className="font-serif text-base font-bold text-gold">{t('recentFailures')}</h2></div>{failures.length === 0 ? <p className="text-sm text-muted-foreground">{t('noFailures')}</p> : <div className="space-y-2">{failures.map((f, i) => <div key={`${f.at}-${i}`} className="rounded-lg border border-border/70 p-3 text-xs"><div className="flex flex-wrap justify-between gap-2"><span>{f.method} {f.path}</span><span className="text-primary">{f.status} · {f.category}</span></div><div className="mt-1 break-all text-muted-foreground">{f.errorCode ?? t('unknownError')} · {f.requestId ?? '-'}</div></div>)}</div>}</section>
        </div>
      )}
    </div>
  )
}

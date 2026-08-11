'use client'

/**
 * 从金丹到道人 - 快捷绑定弹窗
 * 选择一位道人，将金丹绑定给它（可设置权重与服用顺序）
 * 调用 POST /api/v1/agents/:id/pills
 */
import { useState, useEffect } from 'react'
import { useTranslations } from 'next-intl'
import { X, Users, Loader2, Gift, AlertCircle } from 'lucide-react'
import * as agentService from '@/services/agentService'
import type { Agent, Pill } from '@/services/types'

interface BindAgentModalProps {
  pill: Pill
  onClose: () => void
}

export function BindAgentModal({ pill, onClose }: BindAgentModalProps) {
  const t = useTranslations('bindModal')
  const tPill = useTranslations('pill')
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null)
  const [weight, setWeight] = useState(1)
  const [sortOrder, setSortOrder] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  // 加载道人列表
  useEffect(() => {
    let cancelled = false
    agentService.listAgents({ page_size: 100 })
      .then(data => {
        if (!cancelled) setAgents(data.list || [])
      })
      .catch(err => {
        if (!cancelled) setError(err instanceof Error ? err.message : t('errorLoadAgents'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [t])

  /** 执行绑定 */
  const handleBind = async () => {
    if (!selectedAgentId) return
    setSubmitting(true)
    setError(null)
    try {
      await agentService.bindPill(selectedAgentId, pill.id, weight, sortOrder)
      setSuccess(true)
      setTimeout(onClose, 800)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errorBind'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/30 backdrop-blur-sm">
      <div className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300 max-h-[80vh] overflow-y-auto">
        <div className="flex items-center justify-between gap-2 mb-5">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Gift className="w-5 h-5 text-gold shrink-0" />
            <h2 className="text-lg font-serif font-bold text-foreground truncate">
              {tPill('bindCta')}
            </h2>
          </div>
          <button
            aria-label={t('closeModal')}
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <p className="text-xs text-muted-foreground mb-4">
          {t.rich('prompt', {
            name: pill.name,
            gold: (chunks) => <span className="text-gold">{chunks}</span>,
          })}
        </p>

        {loading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="w-6 h-6 text-gold animate-spin" />
          </div>
        ) : agents.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">
            {t('noAgents')}
          </p>
        ) : (
          <div className="space-y-2 mb-4 max-h-60 overflow-y-auto">
            {agents.map(agent => (
              <button
                key={agent.id}
                onClick={() => setSelectedAgentId(agent.id)}
                className={`
                  w-full flex items-center gap-3 p-3 rounded-xl border transition-all text-left min-w-0
                  ${selectedAgentId === agent.id
                    ? 'bg-gold/10 border-gold/40'
                    : 'bg-secondary/70 border-border/70 hover:border-gold/30'
                  }
                `}
              >
                <div className="w-9 h-9 rounded-xl bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-primary-foreground font-serif font-bold shrink-0">
                  {agent.name.charAt(0)}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground truncate">{agent.name}</p>
                  <p className="text-[10px] text-muted-foreground truncate">{agent.model_name}</p>
                </div>
                {selectedAgentId === agent.id && (
                  <span className="w-2 h-2 rounded-full bg-gold shrink-0" />
                )}
              </button>
            ))}
          </div>
        )}

        {agents.length > 0 && (
          <div className="grid grid-cols-2 gap-3 mb-4">
            <div className="min-w-0">
              <label className="dao-label">{t('weightLabel')}</label>
              <input
                type="number"
                min={0}
                max={10}
                step={0.5}
                value={weight}
                onChange={e => setWeight(Math.min(10, Math.max(0, Number(e.target.value))))}
                className="dao-input"
              />
            </div>
            <div className="min-w-0">
              <label className="dao-label">{t('sortOrder')}</label>
              <input
                type="number"
                min={0}
                step={1}
                value={sortOrder}
                onChange={e => setSortOrder(Math.max(0, Math.floor(Number(e.target.value))))}
                className="dao-input"
              />
            </div>
          </div>
        )}

        {error && (
          <div className="flex items-center gap-2 text-xs text-primary mb-3">
            <AlertCircle className="w-3.5 h-3.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <div className="flex items-center gap-3 flex-wrap">
          <button onClick={onClose} className="dao-btn-ghost flex-1 whitespace-nowrap">
            {t('cancel')}
          </button>
          <button
            onClick={handleBind}
            disabled={!selectedAgentId || submitting || success || agents.length === 0}
            className="dao-btn-primary flex-1 disabled:opacity-50 whitespace-nowrap"
          >
            {submitting ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : success ? (
              <Users className="w-4 h-4" />
            ) : (
              <Gift className="w-4 h-4" />
            )}
            {success ? t('success') : t('submit')}
          </button>
        </div>
      </div>
    </div>
  )
}

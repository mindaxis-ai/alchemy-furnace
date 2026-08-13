'use client'

/**
 * 群成员抽屉 - 列表/邀请/踢出
 * 桌面 fixed right-0;H5 bottom sheet(沿用项目 sheet 模式)
 * 所有文案走 useTranslations('groupChat')
 */
import { useState, useEffect } from 'react'
import { useTranslations } from 'next-intl'
import { X, UserMinus, UserPlus, Check } from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import { useChat } from '@/contexts/ChatContext'
import type { ChatSession } from '@/services/types'

export function GroupMembersPanel({
  session,
  open,
  onClose,
}: {
  session: ChatSession
  open: boolean
  onClose: () => void
}) {
  const t = useTranslations('groupChat')
  const { state: agentState, fetchAgents } = useAgent()
  const { inviteMembers, kickMember } = useChat()
  const [showInvite, setShowInvite] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  useEffect(() => {
    if (open) {
      // 抽屉打开时确保拉取最新道人列表(供邀请选择)
      fetchAgents().catch(() => undefined)
    }
  }, [open, fetchAgents])

  if (!open) return null

  const members = session.members || []
  const memberIDs = new Set(members.map(m => m.agent_id))
  const candidates = (agentState.agents || []).filter(
    a => a.status === 'active' && !memberIDs.has(a.id)
  )

  const handleKick = (agentId: string, name: string) => {
    if (window.confirm(t('kickConfirm', { name }))) {
      kickMember(session.id, agentId)
    }
  }

  const toggleSelect = (agentId: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(agentId)) next.delete(agentId)
      else next.add(agentId)
      return next
    })
  }

  const handleInvite = async () => {
    if (selected.size === 0) return
    await inviteMembers(session.id, [...selected])
    setSelected(new Set())
    setShowInvite(false)
  }

  return (
    <>
      {/* 遮罩 */}
      <div
        className="fixed inset-0 z-30 bg-black/30 backdrop-blur-sm animate-in fade-in duration-200"
        onClick={onClose}
        aria-hidden="true"
      />
      {/* 抽屉 */}
      <div
        className={`
          fixed z-40 bg-card border-border/70 shadow-2xl
          inset-y-0 right-0 w-72 border-l
          max-md:inset-x-0 max-md:bottom-0 max-md:top-auto max-md:w-full max-md:rounded-t-2xl max-md:border-l-0 max-md:border-t
          animate-in slide-in-from-right duration-200 max-md:slide-in-from-bottom
        `}
      >
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-border/70">
          <h3 className="text-sm font-serif font-bold text-gold">{t('membersTitle')}</h3>
          <button
            type="button"
            onClick={onClose}
            aria-label={t('close')}
            className="p-1.5 rounded-lg hover:bg-secondary text-muted-foreground"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 邀请按钮 */}
        <div className="p-3 border-b border-border/70">
          {!showInvite ? (
            <button
              type="button"
              onClick={() => setShowInvite(true)}
              className="w-full flex items-center justify-center gap-2 py-2 rounded-lg bg-gold/10 hover:bg-gold/20 text-gold text-sm transition-colors"
            >
              <UserPlus className="w-4 h-4" />
              {t('invite')}
            </button>
          ) : (
            <div className="space-y-2">
              <div className="text-[10px] text-muted-foreground">{t('selectCandidates')}</div>
              <div className="max-h-48 overflow-y-auto space-y-1">
                {candidates.length === 0 ? (
                  <div className="text-xs text-muted-foreground text-center py-3">
                    {t('noCandidates')}
                  </div>
                ) : (
                  candidates.map(a => (
                    <button
                      key={a.id}
                      type="button"
                      onClick={() => toggleSelect(a.id)}
                      className={`
                        w-full flex items-center gap-2 p-2 rounded-lg text-left text-sm transition-colors
                        ${selected.has(a.id)
                          ? 'bg-gold/10 border border-gold/40'
                          : 'bg-muted border border-border/70 hover:border-gold/30'
                        }
                      `}
                    >
                      <div className="w-7 h-7 rounded-full bg-gradient-to-br from-sage to-sage/70 flex items-center justify-center text-white text-xs flex-shrink-0">
                        {a.name.charAt(0)}
                      </div>
                      <span className="flex-1 truncate">{a.name}</span>
                      {selected.has(a.id) && <Check className="w-4 h-4 text-gold" />}
                    </button>
                  ))
                )}
              </div>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => { setShowInvite(false); setSelected(new Set()) }}
                  className="flex-1 py-1.5 rounded-lg bg-muted text-muted-foreground text-xs"
                >
                  {t('cancel')}
                </button>
                <button
                  type="button"
                  onClick={handleInvite}
                  disabled={selected.size === 0}
                  className="flex-1 py-1.5 rounded-lg bg-gold text-foreground text-xs disabled:opacity-40"
                >
                  {t('confirmInvite', { count: selected.size })}
                </button>
              </div>
            </div>
          )}
        </div>

        {/* 成员列表 */}
        <div className="p-3 space-y-1 overflow-y-auto" style={{ maxHeight: 'calc(100vh - 220px)' }}>
          {members.length === 0 ? (
            <div className="text-xs text-muted-foreground text-center py-6">
              {t('noMembers')}
            </div>
          ) : (
            members.map(m => (
              <div
                key={m.agent_id}
                className="flex items-center gap-3 p-2 rounded-lg bg-muted/50 border border-border/70"
              >
                <div className="w-9 h-9 rounded-full bg-gradient-to-br from-gold/60 to-gold/30 flex items-center justify-center text-foreground font-serif font-bold flex-shrink-0">
                  {m.name.charAt(0)}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-foreground truncate">{m.name}</p>
                  <p className="text-[10px] text-muted-foreground">
                    {t('proactivity')}: {m.proactivity}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => handleKick(m.agent_id, m.name)}
                  aria-label={t('kick')}
                  className="p-1.5 rounded-lg hover:bg-primary/10 text-muted-foreground hover:text-primary"
                  title={t('kick')}
                >
                  <UserMinus className="w-4 h-4" />
                </button>
              </div>
            ))
          )}
        </div>
      </div>
    </>
  )
}

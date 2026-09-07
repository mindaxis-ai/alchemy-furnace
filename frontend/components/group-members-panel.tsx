'use client'

/**
 * 群成员抽屉 - 列表/邀请/踢出
 * 桌面 fixed right-0;H5 bottom sheet(沿用项目 sheet 模式)
 * 所有文案走 useTranslations('groupChat')
 */
import { useState, useEffect, useRef } from 'react'
import { useTranslations } from 'next-intl'
import { X, UserMinus, UserPlus, Check } from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import { useChat } from '@/contexts/ChatContext'
import { ProfilePopover } from '@/components/profile-popover'
import { EntityAvatar } from '@/components/avatar/entity-avatar'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { avatarInputMaxLength, validateAvatarField } from '@/lib/avatar-validation'
import type { Agent, ChatSession, GroupMember } from '@/services/types'

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
  const { inviteMembers, kickMember, updateGroupAvatar } = useChat()
  const [showInvite, setShowInvite] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  // 待踢出成员(WKWebView 不实现 window.confirm,用应用内确认框)
  const [kickTarget, setKickTarget] = useState<{ id: string; name: string } | null>(null)
  const [avatarValue, setAvatarValue] = useState(session.avatar || '')
  const [avatarError, setAvatarError] = useState<string | null>(null)
  const [savingAvatar, setSavingAvatar] = useState(false)

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
    setKickTarget({ id: agentId, name })
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

  const handleAvatarSave = async () => {
    const value = avatarValue.trim()
    const validation = validateAvatarField(value)
    if (validation) {
      setAvatarError(validation === 'tooLong' ? t('avatarTooLong') : t('avatarInvalid'))
      return
    }
    setSavingAvatar(true)
    setAvatarError(null)
    const updated = await updateGroupAvatar(session.id, value)
    if (!updated) setAvatarError(t('avatarSaveFailed'))
    setSavingAvatar(false)
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
          fixed z-40 flex flex-col bg-card border-border/70 shadow-2xl
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

        <div className="space-y-2 border-b border-border/70 p-3">
          <label htmlFor="group-avatar" className="text-xs font-medium text-foreground">
            {t('avatarLabel')}
          </label>
          <div className="flex items-center gap-2">
            <EntityAvatar
              name={session.title || t('untitledGroup')}
              src={avatarValue}
              size="md"
              shape="circle"
              alt={t('avatarPreviewAlt')}
            />
            <input
              id="group-avatar"
              value={avatarValue}
              onChange={(event) => {
                setAvatarValue(event.target.value)
                setAvatarError(null)
              }}
              maxLength={avatarInputMaxLength(avatarValue)}
              placeholder={t('avatarPlaceholder')}
              className="min-w-0 flex-1 rounded-lg border border-border/70 bg-muted px-2 py-1.5 text-xs text-foreground outline-none focus:border-gold/60"
            />
          </div>
          {avatarError && <p className="text-xs text-destructive">{avatarError}</p>}
          <div className="flex gap-2">
            <button type="button" onClick={() => setAvatarValue('')} className="flex-1 rounded-lg bg-muted py-1.5 text-xs text-muted-foreground">
              {t('clearAvatar')}
            </button>
            <button type="button" onClick={handleAvatarSave} disabled={savingAvatar} className="flex-1 rounded-lg bg-gold/15 py-1.5 text-xs text-gold disabled:opacity-50">
              {savingAvatar ? t('savingAvatar') : t('saveAvatar')}
            </button>
          </div>
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
                      <EntityAvatar name={a.name} src={a.avatar} size="sm" />
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
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto p-3">
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
                <MemberProfileButton member={m} agent={agentState.agents.find(agent => agent.id === m.agent_id)} />
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

      {/* 踢出确认(应用内对话框,web/桌面行为一致) */}
      {kickTarget && (
        <ConfirmDialog
          description={t('kickConfirm', { name: kickTarget.name })}
          confirmLabel={t('kickConfirmCta')}
          cancelLabel={t('cancel')}
          destructive
          onConfirm={() => {
            kickMember(session.id, kickTarget.id)
            setKickTarget(null)
          }}
          onCancel={() => setKickTarget(null)}
        />
      )}
    </>
  )
}

function MemberProfileButton({ member, agent }: { member: GroupMember; agent?: Agent }) {
  const [open, setOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)
  const profile: Agent = agent || {
    id: member.agent_id,
    name: member.name,
    avatar: member.avatar,
    personality: '',
    model_name: '',
    status: 'active',
    proactivity: member.proactivity,
    created_at: '',
  }

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        onClick={() => setOpen(true)}
        aria-label={`查看 ${member.name} 简介`}
        className="h-8 w-8 flex-shrink-0 overflow-hidden rounded-xl transition hover:ring-2 hover:ring-gold/40"
      >
        <EntityAvatar name={member.name} src={member.avatar} size="sm" />
      </button>
      <ProfilePopover
        kind="agent"
        anchorRef={anchorRef}
        open={open}
        onClose={() => setOpen(false)}
        agent={profile}
      />
    </>
  )
}

'use client'

import { useMemo, useRef, useState } from 'react'
import { useTranslations } from 'next-intl'
import { ChevronDown, Trash2 } from 'lucide-react'
import type { ChatSession } from '@/services/types'
import { TopTabs } from '@/components/interaction/top-tabs'
import { EntityAvatar } from '@/components/avatar/entity-avatar'
import { formatDateTime } from '@/utils/format'
import { groupSingleSessions, sessionKind } from '@/lib/session-presentation'
import { cn } from '@/lib/utils'
import { ConfirmDialog } from '@/components/confirm-dialog'

export interface ConversationDirectoryProps {
  sessions: ChatSession[]
  currentSessionId?: string
  onSelect: (sessionId: string) => void
  onDelete?: (sessionId: string) => Promise<boolean>
  deleteDisabledSessionId?: string
}

/**
 * 会话目录（对谈 / 围炉论道 双 Tab）：
 * - 只消费 props，不请求 API、不读取 AgentContext
 * - 打开具体会话时自动选中该会话所属 Tab，并展开其道人父级
 * - 大厅无当前会话时默认「对谈」；用户手动切换 Tab 只改变筛选
 * - 单聊按 agent_id 分组（agentName 只取 agent_name，绝不显示 UUID）
 * - 道人父级按钮暴露 aria-expanded，键盘可展开/进入会话
 */
export function ConversationDirectory({ sessions, currentSessionId, onSelect, onDelete, deleteDisabledSessionId }: ConversationDirectoryProps) {
  const t = useTranslations('chatView.directory')
  const [activeTab, setActiveTab] = useState<'single' | 'group'>('single')
  const [expandedAgents, setExpandedAgents] = useState<Set<string>>(new Set())
  const [deleteTarget, setDeleteTarget] = useState<ChatSession | null>(null)
  const deletingRef = useRef(false)
  // 只追踪 currentSessionId 变化，sessions 刷新不得重置用户手动选择的 Tab。
  // 渲染期状态调整（React 官方 adjust-state-during-render 模式）：用 prev-state 比较
  // 作为 guard，currentSessionId 变化时同步 Tab 并展开所在道人父级，React 立即用新状态
  // 重渲染；不读 ref（react-hooks/refs），不在 effect 里同步 setState（set-state-in-effect）。
  const [syncedSessionId, setSyncedSessionId] = useState<string | undefined>(undefined)
  if (syncedSessionId !== currentSessionId) {
    setSyncedSessionId(currentSessionId)
    const session = sessions.find(s => s.id === currentSessionId)
    if (!session) {
      setActiveTab('single')
    } else {
      const kind = sessionKind(session)
      setActiveTab(kind)
      if (kind === 'single') {
        setExpandedAgents(prev => new Set(prev).add(session.agent_id))
      }
    }
  }

  const singleGroups = useMemo(() => groupSingleSessions(sessions), [sessions])
  const groupSessions = useMemo(() => sessions.filter(s => sessionKind(s) === 'group'), [sessions])

  const toggleAgent = (agentId: string) => {
    setExpandedAgents(prev => {
      const next = new Set(prev)
      if (next.has(agentId)) next.delete(agentId)
      else next.add(agentId)
      return next
    })
  }

  const selectOnKey = (sessionId: string) => (event: React.KeyboardEvent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onSelect(sessionId)
    }
  }

  const askDelete = (event: React.MouseEvent | React.KeyboardEvent, session: ChatSession) => {
    event.stopPropagation()
    if (session.id !== deleteDisabledSessionId) setDeleteTarget(session)
  }

  const confirmDelete = async () => {
    if (!deleteTarget || !onDelete || deletingRef.current) return
    deletingRef.current = true
    try {
      if (await onDelete(deleteTarget.id)) setDeleteTarget(null)
    } finally {
      deletingRef.current = false
    }
  }

  const deleteButton = (session: ChatSession) => onDelete ? (
    <button
      type="button"
      aria-label={t('deleteAction')}
      title={session.id === deleteDisabledSessionId ? t('deleteStreamingDisabled') : t('deleteAction')}
      disabled={session.id === deleteDisabledSessionId}
      onClick={(event) => askDelete(event, session)}
      onKeyDown={(event) => event.stopPropagation()}
      className="shrink-0 rounded-md p-1.5 text-muted-foreground/70 transition-colors hover:bg-destructive/10 hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-destructive/50 disabled:cursor-not-allowed disabled:opacity-35"
    >
      <Trash2 className="size-3.5" aria-hidden />
    </button>
  ) : null

  return (
    <div className="flex flex-col">
      <TopTabs
        tabs={[
          { key: 'single', label: t('tabs.single') },
          { key: 'group', label: t('tabs.group') },
        ]}
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'single' | 'group')}
      />

      {activeTab === 'single' ? (
        singleGroups.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('emptySingle')}</p>
        ) : (
          <ul className="divide-y divide-border/60">
            {singleGroups.map(group => {
              const expanded = expandedAgents.has(group.agentId)
              const agentLabel = group.agentName || t('unknownAgent')
              return (
                <li key={group.agentId}>
                  <button
                    type="button"
                    aria-expanded={expanded}
                    onClick={() => toggleAgent(group.agentId)}
                    className="flex w-full items-center gap-2 px-3 py-2.5 text-left transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                  >
                    <EntityAvatar name={agentLabel} src={group.agentAvatar} size="sm" shape="circle" />
                    <span className="flex-1 truncate text-sm font-medium text-foreground">{agentLabel}</span>
                    <span className="text-[10px] text-muted-foreground">
                      {t('singleCount', { count: group.sessions.length })}
                    </span>
                    <ChevronDown
                      className={cn('size-4 shrink-0 text-muted-foreground transition-transform duration-300', expanded && 'rotate-180')}
                      aria-hidden
                    />
                  </button>
                  {expanded && (
                    <ul className="pb-2 pl-4 pr-2">
                      {group.sessions.map(s => (
                        <li
                          key={s.id}
                          tabIndex={0}
                          onClick={() => onSelect(s.id)}
                          onKeyDown={selectOnKey(s.id)}
                          className="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                        >
                          <span className="min-w-0 flex-1 truncate">{s.title || t('untitledSingle')}</span>
                          {deleteButton(s)}
                        </li>
                      ))}
                    </ul>
                  )}
                </li>
              )
            })}
          </ul>
        )
      ) : groupSessions.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">{t('emptyGroup')}</p>
      ) : (
        <ul className="divide-y divide-border/60">
          {groupSessions.map(s => (
            <li
              key={s.id}
              tabIndex={0}
              onClick={() => onSelect(s.id)}
              onKeyDown={selectOnKey(s.id)}
              className="cursor-pointer px-3 py-2.5 transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
            >
              <div className="flex items-center gap-2">
                {s.avatar ? (
                  <EntityAvatar name={s.title || t('untitledGroup')} src={s.avatar} size="sm" shape="circle" />
                ) : (
                  <div className="flex shrink-0 -space-x-1.5">
                    {(s.members ?? []).slice(0, 3).map(m => (
                      <EntityAvatar key={m.agent_id} name={m.name} src={m.avatar} size="sm" shape="circle" />
                    ))}
                  </div>
                )}
                <span className="flex-1 truncate text-sm font-medium text-foreground">
                  {s.title || t('untitledGroup')}
                </span>
                <span className="shrink-0 text-[10px] text-muted-foreground">
                  {t('groupMeta', { count: s.members?.length ?? 0 })}
                </span>
                {deleteButton(s)}
              </div>
              <p className="mt-0.5 pl-0 text-[10px] text-muted-foreground">
                {formatDateTime(s.updated_at || s.created_at)}
              </p>
            </li>
          ))}
        </ul>
      )}
      {deleteTarget && (
        <ConfirmDialog
          title={t('deleteTitle')}
          description={t('deleteDescription', { title: deleteTarget.title || t(deleteTarget.type === 'group' ? 'untitledGroup' : 'untitledSingle') })}
          confirmLabel={t('deleteConfirm')}
          cancelLabel={t('deleteCancel')}
          destructive
          onConfirm={() => { void confirmDelete() }}
          onCancel={() => setDeleteTarget(null)}
        />
      )}
    </div>
  )
}

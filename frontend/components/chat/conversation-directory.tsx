'use client'

import { useMemo, useRef, useState } from 'react'
import { useTranslations } from 'next-intl'
import { ChevronDown } from 'lucide-react'
import type { ChatSession } from '@/services/types'
import { TopTabs } from '@/components/interaction/top-tabs'
import { EntityAvatar } from '@/components/avatar/entity-avatar'
import { formatDateTime } from '@/utils/format'
import { groupSingleSessions, sessionKind } from '@/lib/session-presentation'
import { cn } from '@/lib/utils'

export interface ConversationDirectoryProps {
  sessions: ChatSession[]
  currentSessionId?: string
  onSelect: (sessionId: string) => void
}

/**
 * 会话目录（对谈 / 围炉论道 双 Tab）：
 * - 只消费 props，不请求 API、不读取 AgentContext
 * - 打开具体会话时自动选中该会话所属 Tab，并展开其道人父级
 * - 大厅无当前会话时默认「对谈」；用户手动切换 Tab 只改变筛选
 * - 单聊按 agent_id 分组（agentName 只取 agent_name，绝不显示 UUID）
 * - 道人父级按钮暴露 aria-expanded，键盘可展开/进入会话
 */
export function ConversationDirectory({ sessions, currentSessionId, onSelect }: ConversationDirectoryProps) {
  const t = useTranslations('chatView.directory')
  const [activeTab, setActiveTab] = useState<'single' | 'group'>('single')
  const [expandedAgents, setExpandedAgents] = useState<Set<string>>(new Set())
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
                          className="cursor-pointer rounded-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-gold/60"
                        >
                          {s.title || t('untitledSingle')}
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
              </div>
              <p className="mt-0.5 pl-0 text-[10px] text-muted-foreground">
                {formatDateTime(s.updated_at || s.created_at)}
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

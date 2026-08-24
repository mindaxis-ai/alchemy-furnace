'use client'

/**
 * 服丹编排受控组件
 * - 输入输出均为最终数组(value/onChange),自身不触网、不持久化
 * - 行以草稿 key 作 React key;排序仅通过键盘可访问的上移/下移按钮(无拖拽)
 * - 剂量 0-10;越界不在此处拦截,由上层保存校验经 fieldErrors(pills.<key>.weight)回显
 */
import { useTranslations } from 'next-intl'
import { ArrowDown, ArrowUp, FlaskConical, Trash2 } from 'lucide-react'
import type { AgentPillDraftItem, Pill } from '@/services/types'

export interface AgentPillComposerProps {
  /** 当前服丹编排草稿(最终顺序) */
  value: AgentPillDraftItem[]
  /** 任何增/删/改/排序都输出完整新数组 */
  onChange: (next: AgentPillDraftItem[]) => void
  /** 金丹阁全部金丹(名称展示 + 新增选择) */
  pills: Pill[]
  /** 字段级校验错误(键为 pills.<key>.weight,值为机器码如 range,组件侧映射文案) */
  fieldErrors?: Record<string, string>
}

export function AgentPillComposer({
  value,
  onChange,
  pills,
  fieldErrors,
}: AgentPillComposerProps) {
  const t = useTranslations('agent.composer')

  const nameOf = (pillId: string) =>
    pills.find(p => p.id === pillId)?.name ?? t('unknownPill')

  /** 机器码 -> 文案(未知机器码原样展示,不吞错) */
  const formatError = (code: string) => (code === 'range' ? t('weightError') : code)

  const available = pills.filter(p => !value.some(item => item.pill_id === p.id))

  const move = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= value.length) return
    const next = [...value]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    onChange(next)
  }

  const remove = (key: string) => onChange(value.filter(item => item.key !== key))

  const setWeight = (key: string, raw: string) => {
    const parsed = Number(raw)
    const weight = Number.isFinite(parsed) ? parsed : 0
    onChange(value.map(item => (item.key === key ? { ...item, weight } : item)))
  }

  const add = (pillId: string) => {
    if (!pillId || value.some(item => item.pill_id === pillId)) return
    onChange([...value, { key: pillId, pill_id: pillId, weight: 1 }])
  }

  return (
    <div className="space-y-2">
      {value.length === 0 && (
        <p className="text-sm text-muted-foreground">{t('empty')}</p>
      )}

      {value.map((item, index) => {
        const name = nameOf(item.pill_id)
        const errorCode = fieldErrors?.[`pills.${item.key}.weight`]
        return (
          <div
            key={item.key}
            className="rounded-lg border border-border/70 bg-muted p-3"
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gold/15 text-gold">
                <FlaskConical className="h-4 w-4" />
              </span>
              <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                <span className="mr-1 text-xs text-muted-foreground">{index + 1}.</span>
                {name}
              </span>

              <label className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                <input
                  type="number"
                  min={0}
                  max={10}
                  step={0.5}
                  value={item.weight}
                  aria-label={t('weightAria', { name })}
                  onChange={event => setWeight(item.key, event.target.value)}
                  className="dao-input w-20 px-2 py-1 text-sm"
                />
              </label>

              <div className="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  aria-label={t('moveUpAria', { name })}
                  disabled={index === 0}
                  onClick={() => move(index, -1)}
                  className="rounded p-1.5 text-muted-foreground transition-colors hover:bg-gold/10 hover:text-gold disabled:cursor-not-allowed disabled:opacity-30"
                >
                  <ArrowUp className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  aria-label={t('moveDownAria', { name })}
                  disabled={index === value.length - 1}
                  onClick={() => move(index, 1)}
                  className="rounded p-1.5 text-muted-foreground transition-colors hover:bg-gold/10 hover:text-gold disabled:cursor-not-allowed disabled:opacity-30"
                >
                  <ArrowDown className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  aria-label={t('removeAria', { name })}
                  onClick={() => remove(item.key)}
                  className="rounded p-1.5 text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>

            {errorCode && (
              <p role="alert" className="mt-2 break-words text-xs text-primary">
                {formatError(errorCode)}
              </p>
            )}
          </div>
        )
      })}

      {available.length > 0 ? (
        <select
          value=""
          aria-label={t('addLabel')}
          onChange={event => add(event.target.value)}
          className="dao-input text-sm"
        >
          <option value="" disabled>
            {t('addPlaceholder')}
          </option>
          {available.map(pill => (
            <option key={pill.id} value={pill.id}>
              {pill.name}
            </option>
          ))}
        </select>
      ) : (
        value.length > 0 && (
          <p className="text-xs text-sage">{t('allAdded')}</p>
        )
      )}
    </div>
  )
}

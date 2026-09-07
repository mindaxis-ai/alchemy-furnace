'use client'

/**
 * 服用金丹·库存选择弹窗（金丹消耗品重构任务 B3）
 * 固定当前道人(agentId/agentName 由父组件捕获传入，绝不重新拉道人列表)，
 * 从真实可用库存(state=available)分页选择一枚消耗：
 * - listPillItems({page,size:24}) 分页加载，按实例 ID 去重；加载错误与空库存分开展示；
 *   后续页失败保留已加载项与选择并允许重试
 * - activeEffects 的 revision_id 命中视为「已吸收此版本」禁用；已归档丹方产出的
 *   现存可用库存仍可选（只看实例 state，不看丹方归档）
 * - 确认传 {agentId,itemId,weight:1}（无 sortOrder）；itemId 是库存实例 UUID，
 *   不能以 recipe_id/revision_id/effect_id 代替
 * - 提交走 useConsumePillOperation（UUID 幂等）：双击/隐藏到托盘只产生一次 POST；
 *   submitting 禁止关闭；uncertain 可同 key 重试或「稍后核对」退出（父页面保留
 *   pending 提示并禁止保存/再次服用）
 * - 服务端已提交但能力回读失败：显示 syncFailed，只允许「重试同步」（仅再次 GET
 *   effects，绝不再次 POST consume）；可关闭为父页面页内提示
 * - a11y：role=dialog/aria-modal/标题绑定/单选语义/焦点进入与恢复/Tab 不逃出；
 *   键盘（方向键+Enter/Space）与鼠标行为一致
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslations } from 'next-intl'
import { AlertCircle, CheckCircle2, FlaskConical, Loader2, RefreshCw, X } from 'lucide-react'
import { useConsumePillOperation } from '@/hooks/use-consume-pill-operation'
import { listPillItems } from '@/services/pillInventoryService'
import type { AgentEffect, PillItemListItem, PillOperationResult } from '@/services/types'

const PAGE_SIZE = 24

interface ConsumeInventoryPillModalProps {
  agentId: string
  agentName: string
  activeEffects: AgentEffect[]
  onClose(): void
  /** 同步服用结果（父组件：listEffects → reconcile；可重复调用，不得再次 POST consume） */
  onCommitted(result: PillOperationResult): Promise<void>
}

export function ConsumeInventoryPillModal({
  agentId,
  agentName,
  activeEffects,
  onClose,
  onCommitted,
}: ConsumeInventoryPillModalProps) {
  const t = useTranslations('agentDetail.editor')
  const tCommon = useTranslations('common')
  const tk = useCallback(
    (key: string, values?: Record<string, string | number | Date>) => t(`consume.${key}`, values),
    [t],
  )

  const titleId = 'consume-inventory-title'

  // —— 库存分页（仅 available；已消费/弃置实例不进候选池）——
  const [items, setItems] = useState<PillItemListItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [listLoading, setListLoading] = useState(true)
  const [listError, setListError] = useState(false)
  const [moreLoading, setMoreLoading] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)

  // —— 服用执行器（幂等）与能力同步状态 ——
  const [syncFailed, setSyncFailed] = useState(false)
  const committedResultRef = useRef<PillOperationResult | null>(null)

  const absorbedRevisions = useMemo(
    () => new Set(activeEffects.map(e => e.revision_id)),
    [activeEffects],
  )

  // 道人 ID 变化：丢弃旧响应对新页面的 UI 写入，退出弹窗（operation 恢复记录保留在 storage）
  const lastAgentIdRef = useRef(agentId)
  useEffect(() => {
    if (lastAgentIdRef.current !== agentId) {
      lastAgentIdRef.current = agentId
      onClose()
    }
  }, [agentId, onClose])

  /**
   * 分页取可用库存；某页全被历史实例占满时自动继续下一页，直到拿到候选或翻完。
   * 追加模式按实例 ID 去重；失败由调用方展示（保留已加载项与选择）。
   * 用循环而非自递归：自递归的 const 回调会被 react-hooks 静态分析判为「声明前
   * 访问」；语义与递归版等价——首页按入参替换/追加，跳过的后续页总是追加。
   */
  const appendPage = useCallback(async (startPage: number, append: boolean): Promise<boolean> => {
    let pageNo = startPage
    while (true) {
      const res = await listPillItems({ page: pageNo, size: PAGE_SIZE })
      const available = res.items.filter(i => i.state === 'available')
      setItems(prev => {
        const seen = new Set(prev.map(i => i.id))
        const merging = append || pageNo > startPage
        return merging ? [...prev, ...available.filter(i => !seen.has(i.id))] : available
      })
      setTotal(res.total)
      setPage(pageNo)
      const hasMore = pageNo * PAGE_SIZE < res.total
      if (available.length === 0 && res.items.length > 0 && hasMore) {
        pageNo += 1
        continue
      }
      return hasMore
    }
  }, [])

  const loadFirstPage = useCallback(async () => {
    setListLoading(true)
    setListError(false)
    setLoadMoreError(false)
    setItems([])
    setSelectedId(null)
    setPage(0)
    setTotal(0)
    try {
      await appendPage(1, false)
    } catch {
      setListError(true)
    } finally {
      setListLoading(false)
    }
  }, [appendPage])

  // 挂载/道人变化时重置并加载首页（取数副作用；同步 setState 是加载态起点）
  useEffect(() => {
    void loadFirstPage()
  }, [agentId, loadFirstPage])

  const { status, error, submit, retry } = useConsumePillOperation({
    onCommitted: async (input, result) => {
      // 同步失败绝不向执行器抛出（否则 hook 会再次 recover 并重复回调，形成死循环）
      committedResultRef.current = result
      try {
        await onCommitted(result)
        setSyncFailed(false)
        onClose()
      } catch {
        setSyncFailed(true)
      }
    },
  })
  const submitting = status === 'submitting'
  const committed = status === 'committed'
  const uncertain = status === 'uncertain'
  const failed = status === 'error'
  const showSyncFailed = committed && syncFailed

  // 同步重试：仅再次 GET effects，绝不再次 POST consume
  const handleRetrySync = useCallback(async () => {
    if (!committedResultRef.current) return
    try {
      await onCommitted(committedResultRef.current)
      setSyncFailed(false)
      onClose()
    } catch {
      setSyncFailed(true)
    }
  }, [onCommitted, onClose])

  const handleConfirm = () => {
    if (!selectedId || submitting || committed) return
    void submit({ agentId, itemId: selectedId, weight: 1 })
  }

  const handleLoadMore = async () => {
    if (moreLoading) return
    setMoreLoading(true)
    try {
      await appendPage(page + 1, true)
      setLoadMoreError(false)
    } catch {
      setLoadMoreError(true) // 保留已加载项与选择，允许重试
    } finally {
      setMoreLoading(false)
    }
  }

  const hasMore = page > 0 && page * PAGE_SIZE < total

  // 方向键在可用候选中移动单选（键盘与鼠标行为一致）
  const moveSelection = (direction: 1 | -1) => {
    if (submitting) return
    const enabledIds = items.filter(i => !absorbedRevisions.has(i.revision_id)).map(i => i.id)
    if (enabledIds.length === 0) return
    const current = selectedId ? enabledIds.indexOf(selectedId) : -1
    setSelectedId(enabledIds[Math.min(enabledIds.length - 1, Math.max(0, current + direction))])
  }
  const handleGroupKeyDown = (e: React.KeyboardEvent) => {
    const direction =
      e.key === 'ArrowDown' || e.key === 'ArrowRight' ? 1 : e.key === 'ArrowUp' || e.key === 'ArrowLeft' ? -1 : 0
    if (direction === 0 || submitting) return
    e.preventDefault()
    moveSelection(direction)
  }

  // 焦点进入（弹窗容器）与关闭时恢复（回到触发按钮）
  const dialogRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const previouslyFocused =
      document.activeElement instanceof HTMLElement ? document.activeElement : null
    dialogRef.current?.focus()
    return () => previouslyFocused?.focus()
  }, [])

  // Escape 关闭（提交中禁止）与 Tab 不逃出弹窗；方向键在单选候选中移动
  // （焦点初始在弹窗容器上，方向键必须在容器级处理才能到达）
  const handleDialogKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowRight') {
      if (e.target !== e.currentTarget) return // 焦点已在组内时由 handleGroupKeyDown 处理
      e.preventDefault()
      moveSelection(1)
      return
    }
    if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') {
      if (e.target !== e.currentTarget) return
      e.preventDefault()
      moveSelection(-1)
      return
    }
    if (e.key === 'Escape') {
      if (submitting) return
      onClose()
      return
    }
    if (e.key !== 'Tab') return
    const dialog = dialogRef.current
    if (!dialog) return
    const focusables = Array.from(
      dialog.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      ),
    )
    if (focusables.length === 0) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    if (e.shiftKey && (document.activeElement === first || document.activeElement === dialog)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 backdrop-blur-sm">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        onKeyDown={handleDialogKeyDown}
        className="dao-card max-h-[80vh] w-full max-w-md animate-in fade-in duration-300 overflow-y-auto p-6 outline-none"
      >
        <div className="mb-5 flex items-center justify-between gap-2">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <FlaskConical className="h-5 w-5 shrink-0 text-gold" />
            <h2 id={titleId} className="truncate font-serif text-lg font-bold text-foreground">
              {tk('title', { name: agentName })}
            </h2>
          </div>
          <button
            type="button"
            aria-label={tk('close')}
            disabled={submitting}
            onClick={onClose}
            className="shrink-0 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <p className="mb-4 text-xs leading-relaxed text-muted-foreground">{tk('warning')}</p>

        {showSyncFailed ? (
          <div
            role="alert"
            className="mb-4 flex items-start gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3 py-2.5 text-xs text-primary"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{tk('syncFailed')}</span>
          </div>
        ) : uncertain ? (
          <div
            role="alert"
            className="mb-4 flex items-start gap-2 rounded-lg border border-gold/40 bg-gold/10 px-3 py-2.5 text-xs text-gold"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{tk('uncertain')}</span>
          </div>
        ) : failed && error ? (
          <div
            role="alert"
            className="mb-4 flex items-start gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3 py-2.5 text-xs text-primary"
          >
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{error}</span>
          </div>
        ) : null}

        {listLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-gold" />
          </div>
        ) : listError ? (
          <div className="py-4 text-center">
            <p className="mb-3 text-sm text-muted-foreground">{tk('loadFailed')}</p>
            <button type="button" onClick={() => void loadFirstPage()} className="dao-btn-ghost">
              {tCommon('retry')}
            </button>
          </div>
        ) : items.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">{tk('empty')}</p>
        ) : (
          <>
            <div
              role="radiogroup"
              aria-label={tk('title', { name: agentName })}
              onKeyDown={handleGroupKeyDown}
              className="mb-4 max-h-60 space-y-2 overflow-y-auto"
            >
              {items.map(item => {
                const absorbed = absorbedRevisions.has(item.revision_id)
                const locked = submitting || committed
                return (
                  <button
                    key={item.id}
                    type="button"
                    role="radio"
                    aria-checked={selectedId === item.id}
                    disabled={absorbed || locked}
                    onClick={() => setSelectedId(item.id)}
                    className={`
                      flex w-full min-w-0 items-center gap-3 rounded-xl border p-3 text-left transition-all
                      ${
                        selectedId === item.id
                          ? 'border-gold/40 bg-gold/10'
                          : 'border-border/70 bg-secondary/70 hover:border-gold/30'
                      }
                      disabled:cursor-not-allowed disabled:opacity-50
                    `}
                  >
                    <FlaskConical className="h-4 w-4 shrink-0 text-gold" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-foreground">{item.name}</span>
                      {absorbed && (
                        <span className="mt-0.5 flex items-center gap-1 text-[10px] text-muted-foreground">
                          <CheckCircle2 className="h-3 w-3 shrink-0 text-sage" />
                          {tk('duplicate')}
                        </span>
                      )}
                    </span>
                    {selectedId === item.id && <span className="h-2 w-2 shrink-0 rounded-full bg-gold" />}
                  </button>
                )
              })}
            </div>

            {loadMoreError && (
              <div role="alert" className="mb-3 flex items-center justify-between gap-2 text-xs text-primary">
                <span>{tk('loadMoreFailed')}</span>
                <button
                  type="button"
                  onClick={() => void handleLoadMore()}
                  className="shrink-0 text-gold underline-offset-2 hover:underline"
                >
                  {tCommon('retry')}
                </button>
              </div>
            )}
            {hasMore && !loadMoreError && (
              <button
                type="button"
                onClick={() => void handleLoadMore()}
                disabled={moreLoading}
                className="mb-3 flex w-full items-center justify-center gap-1.5 rounded-lg border border-border/70 py-1.5 text-xs text-muted-foreground transition-colors hover:border-gold/40 hover:text-gold disabled:opacity-50"
              >
                {moreLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
                {tk('loadMore')}
              </button>
            )}
          </>
        )}

        <div className="mt-5 flex items-center gap-3">
          {showSyncFailed ? (
            <button
              type="button"
              onClick={() => void handleRetrySync()}
              className="dao-btn-primary flex-1 whitespace-nowrap"
            >
              <RefreshCw className="h-4 w-4" />
              {tk('retrySync')}
            </button>
          ) : uncertain ? (
            <>
              <button type="button" onClick={onClose} className="dao-btn-ghost flex-1 whitespace-nowrap">
                {tk('verifyLater')}
              </button>
              <button
                type="button"
                onClick={() => void retry()}
                className="dao-btn-primary flex-1 whitespace-nowrap"
              >
                <RefreshCw className="h-4 w-4" />
                {tCommon('retry')}
              </button>
            </>
          ) : failed ? (
            <>
              <button type="button" onClick={onClose} className="dao-btn-ghost flex-1 whitespace-nowrap">
                {tk('cancel')}
              </button>
              <button
                type="button"
                onClick={() => void retry()}
                className="dao-btn-primary flex-1 whitespace-nowrap"
              >
                <RefreshCw className="h-4 w-4" />
                {tCommon('retry')}
              </button>
            </>
          ) : (
            <>
              <button
                type="button"
                onClick={onClose}
                disabled={submitting}
                className="dao-btn-ghost flex-1 whitespace-nowrap disabled:opacity-50"
              >
                {tk('cancel')}
              </button>
              <button
                type="button"
                onClick={handleConfirm}
                disabled={!selectedId || submitting}
                className="dao-btn-primary flex-1 whitespace-nowrap disabled:opacity-50"
              >
                {submitting ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : committed ? (
                  <CheckCircle2 className="h-4 w-4" />
                ) : (
                  <FlaskConical className="h-4 w-4" />
                )}
                {submitting ? tk('submitting') : committed ? tk('success') : tk('confirm')}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

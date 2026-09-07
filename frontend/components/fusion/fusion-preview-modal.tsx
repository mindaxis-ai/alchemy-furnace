'use client'

/**
 * 融合炉 - 预览弹窗（融合两阶段的第一阶段）
 * 预览由 POST /fusion/previews 持久化（15 分钟 TTL），不消耗任何材料；
 * 保存时才调用 confirm 原子消耗全部材料并产出新丹。
 * - [编辑] onEdit：仅编辑允许字段（名称/描述），保存后跳新丹详情；不绕回旧 createPill
 * - [保存入库] onSave：只调用 confirm（幂等），不再前端 createPill + deletePill
 * - [关闭] onClose
 */
import { useState } from 'react'
import { useTranslations } from 'next-intl'
import { X, FlaskConical, AlertCircle, Loader2, Edit3, Save, ChevronDown, Info } from 'lucide-react'
import type { FusionPreview, PillItemListItem } from '@/services/types'

interface FusionPreviewModalProps {
  result: FusionPreview
  parents: PillItemListItem[]
  saving: boolean
  onSave: (edited: { name: string; description: string }) => Promise<void> | void
  onEdit: (edited: { name: string; description: string }) => Promise<void> | void
  onClose: () => void
}

export function FusionPreviewModal({
  result,
  parents,
  saving,
  onSave,
  onEdit,
  onClose,
}: FusionPreviewModalProps) {
  const t = useTranslations('fusion.preview')
  const [name, setName] = useState(result.name)
  const [description, setDescription] = useState(result.description)
  const [busy, setBusy] = useState<null | 'save' | 'edit'>(null)

  const handleSave = async () => {
    if (busy || saving) return
    setBusy('save')
    try { await onSave({ name: name.trim() || result.name, description: description.trim() }) } finally { setBusy(null) }
  }
  const handleEdit = async () => {
    if (busy || saving) return
    setBusy('edit')
    try { await onEdit({ name: name.trim() || result.name, description: description.trim() }) } finally { setBusy(null) }
  }
  const operator = result.operator
  const lineageNames = parents.map((p) => p.name).join(' × ')

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/30 backdrop-blur-sm">
      <div className="dao-card w-full max-w-2xl p-6 animate-in fade-in duration-300 max-h-[85vh] overflow-y-auto">
        <div className="flex items-center justify-between gap-3 mb-4">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <FlaskConical className="w-5 h-5 text-gold shrink-0" />
            <h2 className="text-lg font-serif font-bold text-foreground truncate">
              {t('title')}
            </h2>
            <span className="shrink-0 inline-flex items-center gap-1 rounded-full bg-gold/10 px-2.5 py-0.5 text-xs font-medium text-gold">
              {t('operatorLabel')}: {operator.name}
            </span>
          </div>
          <button
            aria-label={t('closeCta')}
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-muted text-muted-foreground hover:text-foreground transition-colors shrink-0"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {result.degraded && (
          <div className="mb-4 flex items-start gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-xs text-primary">
            <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
            <span>{t('degradedWarning')}</span>
          </div>
        )}

        {/* 两阶段契约：预览不消耗材料，保存时才消耗 */}
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-sage/30 bg-sage/5 px-3 py-2 text-xs text-sage">
          <Info className="w-4 h-4 mt-0.5 shrink-0" />
          <span>{t('consumeHint')}</span>
        </div>

        <div className="mb-4 rounded-lg border border-border/60 bg-secondary/30 px-3 py-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{t('lineageLabel')}: </span>
          <span className="text-sage">{lineageNames || '—'}</span>
        </div>

        <div className="space-y-3 mb-4">
          <div>
            <label className="dao-label">{t('nameLabel')}</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="dao-input"
              maxLength={32}
            />
          </div>
          <div>
            <label className="dao-label">{t('descLabel')}</label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="dao-input min-h-[72px] resize-none"
              maxLength={300}
            />
          </div>
        </div>

        <details className="mb-5 group">
          <summary className="cursor-pointer select-none text-xs font-medium text-sage flex items-center gap-1 hover:text-foreground transition-colors">
            <ChevronDown className="w-3.5 h-3.5 transition-transform group-open:rotate-180" />
            {t('schemaTitle')}
          </summary>
          <pre className="mt-2 max-h-64 overflow-auto rounded-lg border border-border/60 bg-muted/40 px-3 py-2 text-[11px] leading-relaxed text-foreground/80 whitespace-pre-wrap break-all">
{JSON.stringify(result.skill_schema, null, 2)}
          </pre>
        </details>

        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={handleEdit}
            disabled={!!busy || saving}
            className="dao-btn-ghost flex-1 min-w-[100px]"
          >
            {busy === 'edit' ? <Loader2 className="w-4 h-4 animate-spin" /> : <Edit3 className="w-4 h-4" />}
            {t('editCta')}
          </button>
          <button
            onClick={handleSave}
            disabled={!!busy || saving}
            className="dao-btn-primary flex-1 min-w-[120px]"
          >
            {(busy === 'save' || saving) ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
            {saving || busy === 'save' ? t('saving') : t('saveCta')}
          </button>
          <button
            onClick={onClose}
            disabled={!!busy || saving}
            className="dao-btn-ghost min-w-[80px]"
          >
            {t('closeCta')}
          </button>
        </div>
      </div>
    </div>
  )
}

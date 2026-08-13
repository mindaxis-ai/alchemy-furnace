'use client'

/**
 * 自动更新对话框(desktop 模式专用)
 * 流程: 触发 check → 显示 changelog + 立即更新 → 下载进度(轮询 progress)→ 重启提示
 * 失败: 显示错误 + 手动下载链接(GitHub release page)
 */
import { useState, useEffect, useCallback } from 'react'
import { useTranslations } from 'next-intl'
import { Download, AlertCircle, CheckCircle2, ExternalLink, X } from 'lucide-react'
import {
  checkUpdate,
  applyUpdate,
  getUpdateProgress,
  type UpdateCheckResult,
} from '@/services/systemService'

type Phase = 'idle' | 'checking' | 'available' | 'downloading' | 'restarting' | 'failed' | 'latest' | 'disabled'

interface UpdateDialogProps {
  /** 关闭回调 */
  onClose: () => void
}

export function UpdateDialog({ onClose }: UpdateDialogProps) {
  const t = useTranslations('update')
  const tAbout = useTranslations('about')
  const [phase, setPhase] = useState<Phase>('idle')
  const [result, setResult] = useState<UpdateCheckResult | null>(null)
  const [pct, setPct] = useState(0)
  const [error, setError] = useState<string | null>(null)

  /** 自动检查更新 */
  const doCheck = useCallback(async () => {
    setPhase('checking')
    setError(null)
    try {
      const r = await checkUpdate()
      setResult(r)
      if (r.notes === '开发构建未启用更新') {
        setPhase('disabled')
      } else if (r.has_update) {
        setPhase('available')
      } else {
        setPhase('latest')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : '检查失败')
      setPhase('failed')
    }
  }, [])

  // mount 时自动检查
  useEffect(() => { doCheck() }, [doCheck])

  // 进度轮询
  useEffect(() => {
    if (phase !== 'downloading') return
    const timer = setInterval(async () => {
      try {
        const { progress } = await getUpdateProgress()
        if (progress < 0) {
          setError(`更新失败 (code ${progress})`)
          setPhase('failed')
          return
        }
        if (progress >= 100 && progress < 110) {
          setPct(100)
          setPhase('restarting')
        } else if (progress >= 110) {
          setPhase('restarting')
        } else {
          setPct(progress)
        }
      } catch {
        // 网络抖动不打断主流程
      }
    }, 1000)
    return () => clearInterval(timer)
  }, [phase])

  /** 触发下载 + 应用 */
  const handleApply = async () => {
    setPhase('downloading')
    setPct(0)
    try {
      await applyUpdate()
      // applyUpdate 返回成功表示 swap 脚本已启动,主进程即将退出
    } catch (e) {
      setError(e instanceof Error ? e.message : '启动更新失败')
      setPhase('failed')
    }
  }

  /** 格式化字节数 */
  const fmtSize = (n: number) => {
    if (n < 1024) return `${n} B`
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
    return `${(n / 1024 / 1024).toFixed(1)} MB`
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="dao-card w-full max-w-md p-6 animate-in fade-in duration-300">
        {/* 头部 */}
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <Download className="w-5 h-5 text-gold" />
            <h2 className="text-lg font-serif font-bold text-gold">{t('found')}</h2>
          </div>
          <button
            aria-label="Close"
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-secondary text-muted-foreground hover:text-foreground transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 状态: 检查中 */}
        {phase === 'checking' && (
          <p className="text-sm text-muted-foreground py-4 text-center">{tAbout('checking')}</p>
        )}

        {/* 状态: 已是最新 */}
        {phase === 'latest' && (
          <div className="py-4 text-center">
            <CheckCircle2 className="w-10 h-10 text-sage mx-auto mb-2" />
            <p className="text-sm text-foreground">{t('latest')}</p>
            <p className="text-xs text-muted-foreground mt-1">{result?.current_version}</p>
          </div>
        )}

        {/* 状态: 开发构建禁用 */}
        {phase === 'disabled' && (
          <div className="py-4 text-center">
            <AlertCircle className="w-10 h-10 text-muted-foreground mx-auto mb-2" />
            <p className="text-sm text-muted-foreground">{t('disabled')}</p>
          </div>
        )}

        {/* 状态: 发现新版本 */}
        {phase === 'available' && result && (
          <>
            <p className="text-sm text-foreground mb-1">
              {t('foundDesc')
                .replace('{latest}', result.latest_version)
                .replace('{size}', fmtSize(result.asset_size))}
            </p>
            <p className="text-xs text-muted-foreground mb-3">
              {result.current_version} → <span className="text-gold font-medium">{result.latest_version}</span>
            </p>
            {result.notes && (
              <details className="mb-4">
                <summary className="text-xs text-sage cursor-pointer hover:underline">
                  {t('releaseNotes')}
                </summary>
                <pre className="mt-2 text-xs text-muted-foreground whitespace-pre-wrap bg-muted p-2 rounded">
                  {result.notes}
                </pre>
              </details>
            )}
            <div className="flex gap-2">
              <button onClick={onClose} className="dao-btn-secondary flex-1">
                {t('later')}
              </button>
              <button onClick={handleApply} className="dao-btn-primary flex-1">
                <Download className="w-4 h-4" />
                {t('update')}
              </button>
            </div>
          </>
        )}

        {/* 状态: 下载中 */}
        {phase === 'downloading' && (
          <div className="py-4">
            <p className="text-sm text-foreground mb-2">{t('downloadingPct').replace('{pct}', String(pct))}</p>
            <div className="h-2 w-full rounded-full bg-muted overflow-hidden">
              <div
                className="h-full bg-gold transition-all duration-300"
                style={{ width: `${pct}%` }}
              />
            </div>
          </div>
        )}

        {/* 状态: 重启中 */}
        {phase === 'restarting' && (
          <div className="py-4 text-center">
            <p className="text-sm text-foreground">{t('restart')}</p>
            <div className="mt-3 h-2 w-full rounded-full bg-muted overflow-hidden">
              <div className="h-full bg-sage animate-pulse" style={{ width: '100%' }} />
            </div>
          </div>
        )}

        {/* 状态: 失败 */}
        {phase === 'failed' && (
          <div className="py-4">
            <div className="flex items-center gap-2 mb-2">
              <AlertCircle className="w-5 h-5 text-red-500" />
              <p className="text-sm text-foreground">{t('failed')}</p>
            </div>
            {error && <p className="text-xs text-muted-foreground mb-3">{error}</p>}
            {result?.page_url && (
              <a
                href={result.page_url}
                target="_blank"
                rel="noopener noreferrer"
                className="dao-btn-secondary inline-flex items-center gap-2"
              >
                <ExternalLink className="w-4 h-4" />
                {t('manual')}
              </a>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

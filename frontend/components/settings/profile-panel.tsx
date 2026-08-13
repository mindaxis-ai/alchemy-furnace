'use client'

/**
 * 设置页「我的简介」面板
 * 飞书式「我的」信息编辑：显示名 + 个人简介。
 * 保存成功时显示「已保存」短暂反馈。
 */
import { useEffect, useState } from 'react'
import { useTranslations } from 'next-intl'
import { User, Check, AlertCircle, Loader2 } from 'lucide-react'
import { useUser } from '@/contexts/UserContext'

const DISPLAY_NAME_MAX = 32
const BIO_MAX = 500

export function ProfilePanel() {
  const t = useTranslations('profile')
  const { profile, loading, error, fetchProfile, updateProfile } = useUser()

  const [displayName, setDisplayName] = useState('')
  const [bio, setBio] = useState('')
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [validationError, setValidationError] = useState<string | null>(null)

  // 拉取 + 填表
  useEffect(() => {
    fetchProfile()
  }, [fetchProfile])

  useEffect(() => {
    if (profile) {
      setDisplayName(profile.display_name || '')
      setBio(profile.bio || '')
    }
  }, [profile])

  const handleSave = async () => {
    setValidationError(null)
    const name = displayName.trim()
    if (!name) {
      setValidationError(t('required'))
      return
    }
    if (name.length > DISPLAY_NAME_MAX) {
      setValidationError(t('tooLong', { max: DISPLAY_NAME_MAX }))
      return
    }
    if (bio.length > BIO_MAX) {
      setValidationError(t('tooLong', { max: BIO_MAX }))
      return
    }
    setSaving(true)
    const updated = await updateProfile({ display_name: name, bio })
    setSaving(false)
    if (updated) {
      setSavedAt(Date.now())
      // 3s 后淡掉
      setTimeout(() => setSavedAt(null), 2500)
    }
  }

  const bioCount = bio.length

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* 页面头部（与道人府/ModelsPanel 同款版式） */}
      <div className="flex items-center gap-3 mb-6 min-w-0">
        <User className="w-6 h-6 text-gold shrink-0" />
        <div className="min-w-0">
          <h1 className="page-title truncate">{t('settingsTitle')}</h1>
          <p className="page-subtitle truncate">{t('settingsSubtitle')}</p>
        </div>
      </div>

      <div className="dao-card p-5 space-y-5">
        {/* 加载/错误态 */}
        {loading && !profile && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin" />
            ...
          </div>
        )}

        {error && (
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-primary/10 border border-primary/30 text-xs text-primary">
            <AlertCircle className="w-4 h-4 shrink-0" />
            {t('loadError')}: {error}
          </div>
        )}

        {/* 显示名 */}
        <div>
          <label htmlFor="display-name" className="block text-xs font-medium text-foreground mb-1.5">
            {t('displayName')}
          </label>
          <input
            id="display-name"
            type="text"
            value={displayName}
            onChange={e => setDisplayName(e.target.value)}
            placeholder={t('displayNamePlaceholder')}
            maxLength={DISPLAY_NAME_MAX + 10}
            className="dao-input w-full"
          />
          <p className="text-[10px] text-muted-foreground mt-1 text-right tabular-nums">
            {displayName.length} / {DISPLAY_NAME_MAX}
          </p>
        </div>

        {/* 简介 */}
        <div>
          <label htmlFor="bio" className="block text-xs font-medium text-foreground mb-1.5">
            {t('bio')}
          </label>
          <textarea
            id="bio"
            value={bio}
            onChange={e => setBio(e.target.value)}
            placeholder={t('bioPlaceholder')}
            rows={5}
            maxLength={BIO_MAX + 50}
            className="dao-input resize-none w-full"
          />
          <p className="text-[10px] text-muted-foreground mt-1 flex items-center justify-between gap-2">
            <span>{t('bioHint')}</span>
            <span className="tabular-nums">{bioCount} / {BIO_MAX}</span>
          </p>
        </div>

        {/* 校验/保存反馈 */}
        {validationError && (
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-primary/10 border border-primary/30 text-xs text-primary">
            <AlertCircle className="w-4 h-4 shrink-0" />
            {validationError}
          </div>
        )}
        {savedAt && (
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-sage/10 border border-sage/30 text-xs text-sage animate-in fade-in duration-200">
            <Check className="w-4 h-4 shrink-0" />
            {t('saved')}
          </div>
        )}

        {/* 保存按钮 */}
        <div className="flex justify-end">
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || loading}
            className="dao-btn-primary inline-flex items-center gap-1.5"
          >
            {saving ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                {t('saving')}
              </>
            ) : (
              <>{t('save')}</>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}

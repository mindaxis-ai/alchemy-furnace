'use client'

import { useRef } from 'react'
import { Upload } from 'lucide-react'

import { EntityAvatar } from '@/components/avatar/entity-avatar'
import type { EntityAvatarProps } from '@/components/avatar/entity-avatar'
import {
  avatarInputMaxLength,
  avatarMaxDataURILen,
  validateAvatarField,
  type AvatarFieldError,
} from '@/lib/avatar-validation'

const allowedImageTypes = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])
// Base64 会膨胀约 4/3；预留 data URI 头部空间后提前拒绝过大的本地文件。
const maxLocalFileBytes = Math.floor((avatarMaxDataURILen - 64) * 3 / 4)

export type AvatarSourceError = AvatarFieldError | 'readFailed'

export function AvatarSourceInput({
  inputId,
  name,
  value,
  onChange,
  onError,
  label,
  linkPlaceholder,
  uploadLabel,
  previewAlt,
  previewSize = 'md',
}: {
  inputId: string
  name: string
  value: string
  onChange: (value: string) => void
  onError: (error: AvatarSourceError | null) => void
  label: string
  linkPlaceholder: string
  uploadLabel: string
  previewAlt: string
  previewSize?: EntityAvatarProps['size']
}) {
  const fileRef = useRef<HTMLInputElement>(null)

  const readLocalFile = (file?: File) => {
    if (!file) return
    if (!allowedImageTypes.has(file.type)) {
      onError('invalid')
      return
    }
    if (file.size > maxLocalFileBytes) {
      onError('tooLong')
      return
    }
    const reader = new FileReader()
    reader.onerror = () => onError('readFailed')
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : ''
      const validation = validateAvatarField(result)
      if (validation) {
        onError(validation)
        return
      }
      onError(null)
      onChange(result)
    }
    reader.readAsDataURL(file)
  }

  return (
    <div className="space-y-2">
      <label htmlFor={inputId} className="block text-xs font-medium text-foreground">
        {label}
      </label>
      <div className="flex items-center gap-2">
        <EntityAvatar name={name} src={value} size={previewSize} shape="circle" alt={previewAlt} />
        <input
          id={inputId}
          value={value}
          onChange={(event) => {
            onError(null)
            onChange(event.target.value)
          }}
          maxLength={avatarInputMaxLength(value)}
          placeholder={linkPlaceholder}
          className="min-w-0 flex-1 rounded-lg border border-border/70 bg-muted px-2 py-1.5 text-xs text-foreground outline-none focus:border-gold/60"
        />
        <input
          ref={fileRef}
          type="file"
          aria-label={`${uploadLabel} file`}
          accept="image/png,image/jpeg,image/webp,image/gif"
          className="hidden"
          onChange={(event) => {
            readLocalFile(event.target.files?.[0])
            event.target.value = ''
          }}
        />
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          className="inline-flex shrink-0 items-center gap-1 rounded-lg border border-border/70 bg-muted px-2.5 py-1.5 text-xs text-muted-foreground hover:border-gold/40 hover:text-gold"
        >
          <Upload className="h-3.5 w-3.5" />
          {uploadLabel}
        </button>
      </div>
    </div>
  )
}

import { post } from './api'
import type { DistillationDraft } from './types'

export function distillNuwa(input: {
  subject: string
  brief: string
  locale: 'zh-CN' | 'en'
}): Promise<DistillationDraft> {
  return post<DistillationDraft>('/distillation/nuwa', input)
}

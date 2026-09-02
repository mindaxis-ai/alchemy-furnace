/** 本地 Prompt 调试偏好。默认关闭，不写入服务端或会话历史。 */
export const PROMPT_DEBUG_STORAGE_KEY = 'alchemy.promptDebug'
export const PROMPT_DEBUG_CHANGE_EVENT = 'alchemy:prompt-debug-change'

export function getPromptDebugEnabled(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(PROMPT_DEBUG_STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

export function setPromptDebugEnabled(enabled: boolean): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(PROMPT_DEBUG_STORAGE_KEY, String(enabled))
    window.dispatchEvent(new CustomEvent<boolean>(PROMPT_DEBUG_CHANGE_EVENT, { detail: enabled }))
  } catch {
    // localStorage 在隐私模式不可用时保持默认关闭。
  }
}

export function subscribePromptDebug(onChange: () => void): () => void {
  const onStorage = (event: StorageEvent) => {
    if (event.key === PROMPT_DEBUG_STORAGE_KEY) onChange()
  }
  window.addEventListener('storage', onStorage)
  window.addEventListener(PROMPT_DEBUG_CHANGE_EVENT, onChange)
  return () => {
    window.removeEventListener('storage', onStorage)
    window.removeEventListener(PROMPT_DEBUG_CHANGE_EVENT, onChange)
  }
}

export const getDefaultPromptDebugEnabled = () => false

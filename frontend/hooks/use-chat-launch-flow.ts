'use client'

import { useCallback, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'

import { useChat } from '@/contexts/ChatContext'
import { chatSessionHref } from '@/lib/chat-route'
import { ApiError } from '@/services/api'

export type LaunchState =
  | { status: 'idle' }
  | { status: 'submitting' }
  | { status: 'error'; message: string; errorCode?: string }

export interface ChatLaunchFlow {
  state: LaunchState
  launchSingle(agentId: string): Promise<boolean>
  launchGroup(agentIds: string[]): Promise<boolean>
  retry(): Promise<boolean>
  reset(): void
}

type LaunchRequest =
  | { type: 'single'; agentId: string }
  | { type: 'group'; agentIds: string[] }

function errorState(error: unknown): LaunchState {
  const message = error instanceof Error ? error.message : '创建会话失败'
  if (error instanceof ApiError && error.errorCode) {
    return { status: 'error', message, errorCode: error.errorCode }
  }
  return { status: 'error', message }
}

export function useChatLaunchFlow(): ChatLaunchFlow {
  const router = useRouter()
  const { createSession, createGroupSession } = useChat()
  const [state, setState] = useState<LaunchState>({ status: 'idle' })
  const inFlightRef = useRef(false)
  const lastFailedRequestRef = useRef<LaunchRequest | null>(null)

  const launch = useCallback(async (request: LaunchRequest): Promise<boolean> => {
    if (inFlightRef.current) return false

    inFlightRef.current = true
    setState({ status: 'submitting' })
    try {
      const session = request.type === 'single'
        ? await createSession(request.agentId)
        : await createGroupSession(request.agentIds)
      lastFailedRequestRef.current = null
      setState({ status: 'idle' })
      router.push(chatSessionHref(session.id))
      return true
    } catch (error) {
      lastFailedRequestRef.current = request
      setState(errorState(error))
      return false
    } finally {
      inFlightRef.current = false
    }
  }, [createGroupSession, createSession, router])

  const launchSingle = useCallback(
    (agentId: string) => launch({ type: 'single', agentId }),
    [launch],
  )

  const launchGroup = useCallback(
    (agentIds: string[]) => launch({ type: 'group', agentIds: [...agentIds] }),
    [launch],
  )

  const retry = useCallback(() => {
    const request = lastFailedRequestRef.current
    return request ? launch(request) : Promise.resolve(false)
  }, [launch])

  const reset = useCallback(() => {
    lastFailedRequestRef.current = null
    setState({ status: 'idle' })
  }, [])

  return { state, launchSingle, launchGroup, retry, reset }
}

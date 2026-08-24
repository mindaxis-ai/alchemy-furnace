'use client'

/**
 * 用户档案 Context（本地单用户部署，后端固定 id=1）
 * 在 Providers 中挂载后,任何组件都能 useUser() 拿到当前用户简介。
 *
 * 头像 popover / 论道列表 / 聊天头像等都依赖这个 context 来展示用户
 * 信息。ProfilePopover(kind=user) 也通过这里读取/更新。
 */
import React, { createContext, useContext, useState, useCallback, useEffect } from 'react'
import * as userService from '@/services/userService'
import type { UserProfile, UpdateUserProfileRequest } from '@/services/userService'

interface UserContextType {
  profile: UserProfile | null
  loading: boolean
  error: string | null
  /** 拉取用户档案（首次进入会自动建默认行） */
  fetchProfile: () => Promise<void>
  /** 更新用户档案;成功时刷新本地 state */
  updateProfile: (data: UpdateUserProfileRequest) => Promise<UserProfile | null>
}

const UserContext = createContext<UserContextType | null>(null)

export function UserProvider({ children }: { children: React.ReactNode }) {
  const [profile, setProfile] = useState<UserProfile | null>(null)
  // 首挂即自动拉取一次（见下方 effect），初始即 loading，避免"未加载"假态闪烁
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchProfile = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await userService.getProfile()
      setProfile(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载用户档案失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const updateProfile = useCallback(async (data: UpdateUserProfileRequest) => {
    setError(null)
    try {
      const updated = await userService.updateProfile(data)
      setProfile(updated)
      return updated
    } catch (e) {
      setError(e instanceof Error ? e.message : '更新用户档案失败')
      return null
    }
  }, [])

  // 首次挂载:自动拉取一次。内联 async loader + 取消保护:
  // setState 只发生在 promise 回调里,且卸载后不再写状态
  useEffect(() => {
    let cancelled = false
    userService.getProfile()
      .then((data) => {
        if (cancelled) return
        setProfile(data)
        setError(null)
      })
      .catch((e) => {
        if (cancelled) return
        setError(e instanceof Error ? e.message : '加载用户档案失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [])

  return (
    <UserContext.Provider value={{ profile, loading, error, fetchProfile, updateProfile }}>
      {children}
    </UserContext.Provider>
  )
}

export function useUser(): UserContextType {
  const ctx = useContext(UserContext)
  if (!ctx) {
    throw new Error('useUser must be used within a UserProvider')
  }
  return ctx
}

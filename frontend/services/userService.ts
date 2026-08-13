/**
 * 用户档案 service
 * 本地/单用户部署,无注册登录,后端固定 id=1
 */
import { get, put } from './api'

/** 用户档案 */
export interface UserProfile {
  display_name: string
  bio: string
  avatar: string
  updated_at: string
}

/** 更新请求(指针语义在 service 层不必要,这里直接用部分类型) */
export interface UpdateUserProfileRequest {
  display_name?: string
  bio?: string
  avatar?: string
}

/** 获取用户档案 */
export function getProfile(): Promise<UserProfile> {
  return get<UserProfile>('/user/profile')
}

/** 更新用户档案 */
export function updateProfile(data: UpdateUserProfileRequest): Promise<UserProfile> {
  return put<UserProfile>('/user/profile', data)
}

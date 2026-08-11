/**
 * 设置页 - 系统配置（tab 化：模型管理 / 关于）
 * server 框架 + Suspense 包 client SettingsTabs（useSearchParams 在 output:export 下必须有 Suspense boundary）
 *
 * 页面不提供外层容器/页头：tab 条即页面级导航,各 tab 内容自带
 * 标准页头(icon + h1 + 操作位)与容器,与道人府/金丹阁同一版式。
 */
import { Suspense } from 'react'
import { SettingsTabs } from './settings-tabs'

export default function SettingsPage() {
  return (
    <Suspense fallback={null}>
      <SettingsTabs />
    </Suspense>
  )
}

/**
 * 设置页 - 系统配置（tab 化：模型管理 / 关于）
 * server 框架 + Suspense 包 client SettingsTabs（useSearchParams 在 output:export 下必须有 Suspense boundary）
 */
import { Suspense } from 'react'
import { Settings2 } from 'lucide-react'
import { SettingsTabs } from './settings-tabs'

export default function SettingsPage() {
  return (
    <div className="mx-auto max-w-6xl px-4 pt-8 sm:px-6">
      {/* 页面头部 */}
      <div className="flex items-center gap-3 mb-6">
        <Settings2 className="w-6 h-6 text-gold" />
        <div>
          <h1 className="page-title">设置</h1>
          <p className="page-subtitle">配置系统参数</p>
        </div>
      </div>
      <Suspense fallback={null}>
        <SettingsTabs />
      </Suspense>
    </div>
  )
}

'use client'

/**
 * /models 已合并入 /settings（模型管理 tab）
 * client 端重定向：output:export 静态导出不支持服务端 redirect()
 * router.replace 不产生 history 记录；兜底 Link 供 JS 禁用/慢网点击
 */
import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'

export default function ModelsRedirect() {
  const router = useRouter()

  useEffect(() => {
    router.replace('/settings')
  }, [router])

  return (
    <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 text-center">
      <p className="text-muted-foreground">
        模型管理已合并到设置页，
        <Link href="/settings" className="text-gold hover:text-gold/80 mx-1">
          点击前往
        </Link>
      </p>
    </div>
  )
}

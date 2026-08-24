'use client'

/**
 * 桌面壳行为守卫 (T1)
 * - 禁浏览器右键菜单(仅 UI 区域;输入/内容/可选区放行)
 * - 禁图片/链接原生拖拽
 * - 仅 is-desktop 生效;web/H5 不挂任何监听
 * T4 将扩展快捷键守卫
 */
import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { isDesktop } from '@/services/api'

const ALLOW = 'input, textarea, [contenteditable="true"], .md-selectable'

export default function DesktopGuards() {
  const router = useRouter()
  useEffect(() => {
    if (!isDesktop()) return
    const onContextMenu = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest(ALLOW)) return
      e.preventDefault()
    }
    const onDragStart = (e: DragEvent) => {
      if ((e.target as HTMLElement).closest(ALLOW)) return
      e.preventDefault()
    }
    window.addEventListener('contextmenu', onContextMenu)
    window.addEventListener('dragstart', onDragStart)
    // T4 快捷键: ⌘, 进设置;⌘N 弹新建会话(由 chat-view 监听 alchemy:new-session 事件处理)
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return
      if (e.key === ',') {
        e.preventDefault()
        router.push('/settings')
      } else if (e.key === 'n' || e.key === 'N') {
        e.preventDefault()
        window.dispatchEvent(new CustomEvent('alchemy:new-session'))
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('contextmenu', onContextMenu)
      window.removeEventListener('dragstart', onDragStart)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [router])
  return null
}

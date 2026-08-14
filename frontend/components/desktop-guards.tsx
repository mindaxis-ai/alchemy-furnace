'use client'

/**
 * 桌面壳行为守卫 (T1)
 * - 禁浏览器右键菜单(仅 UI 区域;输入/内容/可选区放行)
 * - 禁图片/链接原生拖拽
 * - 仅 is-desktop 生效;web/H5 不挂任何监听
 * T4 将扩展快捷键守卫
 */
import { useEffect } from 'react'
import { isDesktop } from '@/services/api'

const ALLOW = 'input, textarea, [contenteditable="true"], .md-selectable'

export default function DesktopGuards() {
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
    return () => {
      window.removeEventListener('contextmenu', onContextMenu)
      window.removeEventListener('dragstart', onDragStart)
    }
  }, [])
  return null
}

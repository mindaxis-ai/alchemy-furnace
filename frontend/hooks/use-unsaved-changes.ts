'use client'

/**
 * 未保存更改离开保护
 * active 为 true 时：
 * - beforeunload 阻止浏览器关闭/刷新（触发浏览器原生确认，自定义文案被浏览器忽略）
 * - 捕获阶段拦截站内 <a href="/..."> 点击：window.confirm 取消则阻止导航
 * message 用于站内返回的确认文案。active 转 false 时移除全部监听。
 */
import { useEffect, useRef } from 'react'

export function useUnsavedChanges(active: boolean, message: string): void {
  const messageRef = useRef(message)
  // 最新文案存入 ref(在 effect 中更新,避免渲染期写 ref)
  useEffect(() => {
    messageRef.current = message
  }, [message])

  useEffect(() => {
    if (!active) return

    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }

    const onClick = (event: MouseEvent) => {
      // 已被其它处理器取消、非左键、带修饰键(新标签/新窗口打开)的点击不拦截
      if (event.defaultPrevented) return
      if (event.button !== 0) return
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
      const target = event.target
      const anchor =
        target instanceof Element ? target.closest<HTMLAnchorElement>('a[href^="/"]') : null
      if (!anchor) return
      if (window.confirm(messageRef.current)) return
      event.preventDefault()
      event.stopPropagation()
    }

    window.addEventListener('beforeunload', onBeforeUnload)
    // 捕获阶段:先于站内 Link 自身的导航处理运行
    document.addEventListener('click', onClick, true)
    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload)
      document.removeEventListener('click', onClick, true)
    }
  }, [active])
}

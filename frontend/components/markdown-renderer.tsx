'use client'

/**
 * Markdown 渲染组件
 * 支持代码高亮、表格、列表等常见 Markdown 语法
 * 使用 react-markdown + remark-gfm + highlight.js(浅色主题)
 *
 * 流式性能:
 *   - streaming=true 时: 纯文本逐字显示(跳过 react-markdown 解析 + hljs 全局扫描)
 *     这避免了每 chunk 都 O(n) 重新解析,让 LLM 流式逐字进入 UI 跟得上
 *   - streaming=false 时: 完整 markdown 渲染(代码高亮、表格等)
 */
import { useEffect, useRef } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import remarkBreaks from 'remark-breaks'
import hljs from 'highlight.js'
import 'highlight.js/styles/github.min.css'

interface MarkdownRendererProps {
  content: string
  /**
   * 是否处于流式输出中
   * - true:  纯文本逐字显示(零解析开销,流式体感顺滑)
   * - false: 完整 markdown 渲染
   */
  streaming?: boolean
}

export function MarkdownRenderer({ content, streaming = false }: MarkdownRendererProps) {
  // 代码高亮的容器 ref — 只在流结束后 highlight 当前容器(而非 hljs.highlightAll 全局扫描)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (streaming) return // 流中不做高亮,等流结束切到 markdown 模式时再做
    if (!containerRef.current) return
    // 只高亮当前容器内的 code 块(避免 hljs.highlightAll() 扫整页)
    containerRef.current.querySelectorAll('pre code').forEach((block) => {
      // 已高亮过则跳过(react-markdown 重渲可能不重置 class)
      if (block.classList.contains('hljs')) {
        try {
          hljs.highlightElement(block as HTMLElement)
        } catch {
          /* 忽略高亮异常,回退到纯文本 */
        }
      }
    })
  }, [content, streaming])

  if (streaming) {
    // 流式模式: 纯文本(保留换行),无 markdown 解析,光标放在最末由父组件光标 span 实现
    return (
      <div ref={containerRef} className="whitespace-pre-wrap break-words leading-relaxed">
        {content}
      </div>
    )
  }

  return (
    <div ref={containerRef} className="markdown-body">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkBreaks]}
        components={{
          // 代码块
          code({ className, children, ...props }) {
            const match = /language-(\w+)/.exec(className || '')
            const language = match ? match[1] : ''
            const isInline = !className

            if (isInline) {
              return (
                <code className="bg-muted text-sage px-1.5 py-0.5 rounded text-xs" {...props}>
                  {children}
                </code>
              )
            }

            return (
              <div className="my-2 rounded-lg overflow-hidden border border-border/70">
                {/* 代码块头部 */}
                <div className="flex items-center justify-between px-3 py-1.5 bg-paper-deep border-b border-border/70">
                  <span className="text-[10px] text-muted-foreground uppercase">
                    {language || 'text'}
                  </span>
                </div>
                <pre className="m-0 p-0">
                  <code className={`hljs ${language}`}>
                    {String(children).replace(/\n$/, '')}
                  </code>
                </pre>
              </div>
            )
          },
          // 引用块
          blockquote({ children }) {
            return (
              <blockquote className="border-l-2 border-gold/50 bg-gold/5 pl-4 py-1 pr-2 my-2 rounded-r">
                {children}
              </blockquote>
            )
          },
          // 表格
          table({ children }) {
            return (
              <div className="overflow-x-auto my-2">
                <table className="w-full text-xs border-collapse">
                  {children}
                </table>
              </div>
            )
          },
          thead({ children }) {
            return <thead className="bg-muted">{children}</thead>
          },
          th({ children }) {
            return (
              <th className="text-left px-3 py-2 text-gold font-medium border border-border/70">
                {children}
              </th>
            )
          },
          td({ children }) {
            return (
              <td className="px-3 py-2 text-foreground/80 border border-border/70">
                {children}
              </td>
            )
          },
          // 列表
          ul({ children }) {
            return <ul className="list-disc list-inside space-y-0.5 my-1">{children}</ul>
          },
          ol({ children }) {
            return <ol className="list-decimal list-inside space-y-0.5 my-1">{children}</ol>
          },
          // 链接
          a({ children, href }) {
            return (
              <a href={href} target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">
                {children}
              </a>
            )
          },
          // 段落
          p({ children }) {
            return <p className="my-1.5 leading-relaxed">{children}</p>
          },
          // 标题
          h1({ children }) {
            return <h1 className="text-lg font-bold text-foreground mt-4 mb-2 font-serif">{children}</h1>
          },
          h2({ children }) {
            return <h2 className="text-base font-bold text-foreground mt-3 mb-2 font-serif">{children}</h2>
          },
          h3({ children }) {
            return <h3 className="text-sm font-bold text-foreground mt-2 mb-1 font-serif">{children}</h3>
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

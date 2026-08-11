'use client'

/**
 * 聊天消息气泡组件 - 浅色宣纸卷轴风
 * 用户消息: 右侧，朱砂红边框
 * AI 消息: 左侧，金色边框，卷轴风格
 * 支持 markdown 渲染（RAG 引用来源展示已移除）
 */
import { useTranslations } from 'next-intl'
import { User, Bot, TriangleAlert, CircleStop } from 'lucide-react'
import type { ChatMessage as ChatMessageType } from '@/services/types'
import { MarkdownRenderer } from '@/components/markdown-renderer'

interface ChatMessageProps {
  message: ChatMessageType
  /** 是否正在流式输出中 */
  streaming?: boolean
}

export function ChatMessage({ message, streaming = false }: ChatMessageProps) {
  const t = useTranslations('chatMessage')
  const isUser = message.role === 'user'

  return (
    <div className={`
      flex gap-3 md:gap-4 min-w-0
      ${isUser ? 'flex-row-reverse' : 'flex-row'}
      animate-in fade-in duration-300
    `}>
      {/* 头像 */}
      <div className={`
        shrink-0 w-9 h-9 md:w-10 md:h-10 rounded-full flex items-center justify-center
        ${isUser
          ? 'bg-primary/10 text-primary border border-primary/30'
          : 'bg-gold/15 text-gold border border-gold/30'
        }
      `}>
        {isUser ? <User className="w-5 h-5" /> : <Bot className="w-5 h-5" />}
      </div>

      {/* 消息内容 */}
      <div className={`
        flex-1 max-w-[85%] md:max-w-[75%] min-w-0
        ${isUser ? 'text-right' : 'text-left'}
      `}>
        {/* 角色标签 */}
        <span className={`
          inline-block text-[10px] mb-1.5 px-2 py-0.5 rounded-full whitespace-nowrap
          ${isUser
            ? 'bg-primary/10 text-primary/70'
            : 'bg-gold/10 text-gold/80'
          }
        `}>
          {isUser ? t('userLabel') : t('assistantLabel')}
        </span>

        {/* 消息气泡 */}
        <div className={`
          relative inline-block text-left max-w-full
          px-4 py-3 rounded-2xl
          ${isUser
            ? 'bg-primary/5 border border-primary/30 rounded-tr-sm'
            : 'bg-card/90 border border-gold/30 rounded-tl-sm'
          }
        `}>
          {/* 卷轴装饰（仅 AI 消息） */}
          {!isUser && (
            <>
              <div className="absolute -left-1 top-2 bottom-2 w-1 bg-gradient-to-b from-gold/60 via-gold/40 to-gold/60 rounded-full" />
              <div className="absolute -right-1 top-2 bottom-2 w-1 bg-gradient-to-b from-gold/60 via-gold/40 to-gold/60 rounded-full" />
            </>
          )}

          {/* 消息内容 - Markdown 渲染 */}
          <div className={`${isUser ? '' : 'pl-2 pr-2'} min-w-0 break-words`}>
            {isUser ? (
              <p className="text-sm text-foreground whitespace-pre-wrap leading-relaxed">
                {message.content}
              </p>
            ) : (
              <MarkdownRenderer content={message.content} />
            )}
          </div>

          {/* 流式输出光标 */}
          {streaming && !isUser && (
            <span className="inline-block w-2 h-4 bg-gold ml-1 animate-pulse" />
          )}
        </div>

        {/* 状态标记（仅 AI 消息） */}
        {!isUser && (message.incomplete || message.stopped) && (
          <div className="flex items-center gap-3 mt-1.5 pl-1 flex-wrap">
            {message.incomplete && (
              <span className="flex items-center gap-1 text-[10px] text-gold/80 whitespace-nowrap">
                <TriangleAlert className="w-3 h-3 shrink-0" />
                {t('incomplete')}
              </span>
            )}
            {message.stopped && (
              <span className="flex items-center gap-1 text-[10px] text-muted-foreground whitespace-nowrap">
                <CircleStop className="w-3 h-3 shrink-0" />
                {t('stopped')}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

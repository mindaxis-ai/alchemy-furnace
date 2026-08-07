/**
 * 聊天消息气泡组件 - 道教卷轴风格
 * 用户消息: 右侧，朱砂红边框
 * AI 消息: 左侧，金色边框，卷轴风格
 * 支持 markdown 渲染（RAG 引用来源展示已移除）
 */
import { User, Bot } from 'lucide-react'
import type { ChatMessage as ChatMessageType } from '@/services/types'
import MarkdownRenderer from './MarkdownRenderer'

interface ChatMessageProps {
  message: ChatMessageType
  /** 是否正在流式输出中 */
  streaming?: boolean
}

export default function ChatMessage({ message, streaming = false }: ChatMessageProps) {
  const isUser = message.role === 'user'

  return (
    <div className={`
      flex gap-3 md:gap-4
      ${isUser ? 'flex-row-reverse' : 'flex-row'}
      animate-fade-in
    `}>
      {/* 头像 */}
      <div className={`
        flex-shrink-0 w-9 h-9 md:w-10 md:h-10 rounded-full flex items-center justify-center
        ${isUser
          ? 'bg-cinnabar-500/20 text-cinnabar-400 border border-cinnabar-500/30'
          : 'bg-gold-500/20 text-gold-400 border border-gold-500/30'
        }
      `}>
        {isUser ? <User className="w-5 h-5" /> : <Bot className="w-5 h-5" />}
      </div>

      {/* 消息内容 */}
      <div className={`
        flex-1 max-w-[85%] md:max-w-[75%]
        ${isUser ? 'text-right' : 'text-left'}
      `}>
        {/* 角色标签 */}
        <span className={`
          inline-block text-[10px] mb-1.5 px-2 py-0.5 rounded-full
          ${isUser
            ? 'bg-cinnabar-500/10 text-cinnabar-400/70'
            : 'bg-gold-500/10 text-gold-400/70'
          }
        `}>
          {isUser ? '道友' : '道人'}
        </span>

        {/* 消息气泡 */}
        <div className={`
          relative inline-block text-left
          px-4 py-3 rounded-2xl
          ${isUser
            ? 'bg-cinnabar-500/10 border border-cinnabar-500/30 rounded-tr-sm'
            : 'bg-ink-800/80 border border-gold-500/30 rounded-tl-sm scroll-container'
          }
          ${streaming && !isUser ? 'animate-pulse-glow' : ''}
        `}>
          {/* 卷轴装饰（仅 AI 消息） */}
          {!isUser && (
            <>
              <div className="absolute -left-1 top-2 bottom-2 w-1 bg-gradient-to-b from-bronze-500 via-bronze-600 to-bronze-500 rounded-full" />
              <div className="absolute -right-1 top-2 bottom-2 w-1 bg-gradient-to-b from-bronze-500 via-bronze-600 to-bronze-500 rounded-full" />
            </>
          )}

          {/* 消息内容 - Markdown 渲染 */}
          <div className={`${isUser ? '' : 'pl-2 pr-2'}`}>
            {isUser ? (
              <p className="text-sm text-rice-paper-100 whitespace-pre-wrap leading-relaxed">
                {message.content}
              </p>
            ) : (
              <MarkdownRenderer content={message.content} />
            )}
          </div>

          {/* 流式输出光标 */}
          {streaming && !isUser && (
            <span className="inline-block w-2 h-4 bg-gold-400 ml-1 animate-pulse" />
          )}
        </div>
      </div>
    </div>
  )
}

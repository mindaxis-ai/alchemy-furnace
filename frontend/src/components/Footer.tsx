/**
 * 底部信息栏组件 - 道教风格
 * 显示项目标语、开源链接等信息
 */
import { Flame, Heart } from 'lucide-react'

export default function Footer() {
  return (
    <footer className="bg-ink-800/60 border-t border-bronze-600/20 mt-auto">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <div className="flex flex-col md:flex-row items-center justify-between gap-4">
          {/* 左侧：Logo + 标语 */}
          <div className="flex flex-col items-center md:items-start gap-2">
            <div className="flex items-center gap-2">
              <Flame className="w-4 h-4 text-cinnabar-400" />
              <span className="text-sm font-serif text-gold-300/80">炼丹炉</span>
            </div>
            <p className="text-xs text-ink-400 tracking-wider">
              道法自然 · 炼丹铸智
            </p>
          </div>

          {/* 中间：技术栈标签 */}
          <div className="flex flex-wrap items-center justify-center gap-2">
            {['React 18', 'TypeScript', 'Tailwind CSS', '金丹化性'].map(tag => (
              <span
                key={tag}
                className="px-2 py-0.5 text-[10px] rounded-full bg-jade-500/10 text-jade-400/70 border border-jade-500/20"
              >
                {tag}
              </span>
            ))}
          </div>

          {/* 右侧：开源信息 */}
          <div className="flex items-center gap-1 text-xs text-ink-400">
            <span>用</span>
            <Heart className="w-3 h-3 text-cinnabar-400 fill-cinnabar-400" />
            <span>炼制</span>
            <span className="mx-1 text-ink-600">·</span>
            <a
              href="https://github.com"
              target="_blank"
              rel="noopener noreferrer"
              className="text-gold-400/60 hover:text-gold-400 transition-colors"
            >
              开源项目
            </a>
          </div>
        </div>

        {/* 底部分隔 + 版权 */}
        <div className="mt-4 pt-4 border-t border-ink-700/50 text-center">
          <p className="text-[10px] text-ink-500">
            炼丹炉 - 以道教文化为灵感的金丹化性（Skill-Persona Alchemy）系统
          </p>
        </div>
      </div>
    </footer>
  )
}

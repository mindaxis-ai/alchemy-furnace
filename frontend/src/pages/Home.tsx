/**
 * 首页 - 炼丹炉主殿
 * 大气的道教风格视觉，炼丹炉 CSS 动画效果
 * 功能入口: 金丹阁 | 道人府 | 炼丹室
 * 响应式: H5 显示为竖排卡片
 */
import { Link } from 'react-router-dom'
import {
  Flame,
  CircleDot,
  Users,
  MessageSquare,
  Sparkles,
  BookOpen,
  Compass,
  Zap,
} from 'lucide-react'

/** 功能入口配置 */
const features = [
  {
    path: '/pills',
    title: '金丹阁',
    subtitle: '语言模式金丹',
    description: '炼制金丹，铸就性情。以表达 DNA、心智模型与决策启发式塑造语言风格。',
    icon: CircleDot,
    color: 'from-gold-500/20 to-gold-700/10',
    iconColor: 'text-gold-400',
    borderColor: 'border-gold-500/30 hover:border-gold-400/60',
    glowColor: 'hover:shadow-gold-400/10',
  },
  {
    path: '/agents',
    title: '道人府',
    subtitle: 'AI Agent 管理',
    description: '招募道人，各怀绝技。为每个道人赋予独特的性格与智慧。',
    icon: Users,
    color: 'from-jade-500/20 to-jade-700/10',
    iconColor: 'text-jade-400',
    borderColor: 'border-jade-500/30 hover:border-jade-400/60',
    glowColor: 'hover:shadow-jade-400/10',
  },
  {
    path: '/chat',
    title: '论道',
    subtitle: '金丹化性对话',
    description: '选择道人，开始论道。让 AI 以金丹化性后的性情与你对谈。',
    icon: MessageSquare,
    color: 'from-cinnabar-500/20 to-cinnabar-700/10',
    iconColor: 'text-cinnabar-400',
    borderColor: 'border-cinnabar-500/30 hover:border-cinnabar-400/60',
    glowColor: 'hover:shadow-cinnabar-400/10',
  },
]

/** 统计数据 */
const stats = [
  { label: '金丹', value: '12', icon: CircleDot },
  { label: '道人', value: '5', icon: Users },
  { label: '论道', value: '128', icon: MessageSquare },
]

export default function Home() {
  return (
    <div className="space-y-8 md:space-y-12 pb-8">
      {/* ========== 炼丹炉视觉区域 ========== */}
      <section className="relative flex flex-col items-center justify-center py-8 md:py-12">
        {/* 背景光晕 */}
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div className="w-64 h-64 md:w-96 md:h-96 rounded-full bg-cinnabar-500/5 blur-3xl animate-glow" />
        </div>

        {/* 炼丹炉动画 */}
        <div className="relative mb-6 md:mb-8">
          {/* 外层光环 */}
          <div className="absolute inset-0 -m-6 md:-m-8">
            <div className="w-full h-full rounded-full border border-gold-500/10 animate-spin-slow" />
          </div>
          <div className="absolute inset-0 -m-3 md:-m-4">
            <div className="w-full h-full rounded-full border border-dashed border-cinnabar-500/15 animate-spin-slow" style={{ animationDirection: 'reverse', animationDuration: '12s' }} />
          </div>

          {/* 炼丹炉主体 */}
          <div className="relative w-28 h-28 md:w-36 md:h-36 rounded-full bg-gradient-to-b from-cinnabar-500/20 via-cinnabar-600/30 to-ink-800/80 border-2 border-gold-500/30 flex items-center justify-center glow-gold">
            {/* 火焰图标 */}
            <Flame className="w-12 h-12 md:w-16 md:h-16 text-cinnabar-400 animate-glow" />

            {/* 内部光点 */}
            <div className="absolute inset-4 rounded-full bg-gold-400/10 animate-pulse" />
          </div>

          {/* 烟雾粒子 */}
          <div className="absolute -top-4 left-1/2 -translate-x-1/2">
            <div className="smoke-particle" style={{ left: '-10px', animationDelay: '0s' }} />
            <div className="smoke-particle" style={{ left: '5px', animationDelay: '0.5s' }} />
            <div className="smoke-particle" style={{ left: '-5px', animationDelay: '1s' }} />
            <div className="smoke-particle" style={{ left: '10px', animationDelay: '1.5s' }} />
          </div>
        </div>

        {/* 标题 */}
        <h1 className="text-3xl md:text-5xl font-serif font-black text-gold-300 tracking-widest text-center mb-3">
          炼丹炉
        </h1>
        <p className="text-sm md:text-base text-ink-400 text-center max-w-md px-4">
          以道教炼丹为灵感的金丹化性系统：炼语言模式之丹，铸 AI 道人之性
        </p>

        {/* 标语 */}
        <div className="flex items-center gap-2 mt-4 text-gold-500/40 text-xs tracking-[0.3em]">
          <Sparkles className="w-3 h-3" />
          <span>道法自然 · 炼丹铸智</span>
          <Sparkles className="w-3 h-3" />
        </div>
      </section>

      {/* ========== 统计数据栏 ========== */}
      <section className="grid grid-cols-3 gap-3 md:gap-4 max-w-3xl mx-auto">
        {stats.map(stat => {
          const Icon = stat.icon
          return (
            <div
              key={stat.label}
              className="dao-card flex flex-col items-center py-4 px-3 text-center"
            >
              <Icon className="w-5 h-5 text-gold-400/60 mb-2" />
              <span className="text-2xl font-serif font-bold text-gold-300">{stat.value}</span>
              <span className="text-xs text-ink-400 mt-0.5">{stat.label}</span>
            </div>
          )
        })}
      </section>

      {/* ========== 功能入口卡片 ========== */}
      <section>
        <div className="flex items-center gap-3 mb-5 md:mb-6">
          <Compass className="w-5 h-5 text-gold-400" />
          <h2 className="page-title text-xl">修行之路</h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6">
          {features.map(feature => {
            const Icon = feature.icon
            return (
              <Link
                key={feature.path}
                to={feature.path}
                className={`
                  group relative overflow-hidden rounded-xl
                  bg-gradient-to-br ${feature.color}
                  border ${feature.borderColor}
                  p-5 md:p-6
                  transition-all duration-300
                  hover:scale-[1.02] hover:-translate-y-0.5
                  ${feature.glowColor} hover:shadow-lg
                `}
              >
                {/* 背景装饰 */}
                <div className="absolute top-0 right-0 w-24 h-24 bg-white/3 rounded-full -translate-y-1/2 translate-x-1/2 group-hover:scale-150 transition-transform duration-500" />

                {/* 内容 */}
                <div className="relative">
                  <div className={`
                    w-12 h-12 rounded-xl flex items-center justify-center mb-4
                    bg-ink-800/50 ${feature.iconColor}
                    group-hover:scale-110 transition-transform duration-300
                  `}>
                    <Icon className="w-6 h-6" />
                  </div>

                  <div className="mb-2">
                    <h3 className="text-lg font-serif font-bold text-rice-paper-100 group-hover:text-gold-300 transition-colors">
                      {feature.title}
                    </h3>
                    <span className="text-xs text-ink-400">{feature.subtitle}</span>
                  </div>

                  <p className="text-sm text-ink-400 leading-relaxed">
                    {feature.description}
                  </p>

                  {/* 箭头 */}
                  <div className="flex items-center gap-1 mt-4 text-xs text-gold-400/50 group-hover:text-gold-400 transition-colors">
                    <Zap className="w-3.5 h-3.5" />
                    <span>进入{feature.title}</span>
                  </div>
                </div>
              </Link>
            )
          })}
        </div>
      </section>

      {/* ========== 流程说明 ========== */}
      <section>
        <div className="flex items-center gap-3 mb-5 md:mb-6">
          <BookOpen className="w-5 h-5 text-gold-400" />
          <h2 className="page-title text-xl">炼丹之道</h2>
        </div>

        <div className="dao-card p-5 md:p-6">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4 md:gap-6">
            {[
              { step: '壹', title: '炼制金丹', desc: '编辑表达 DNA 与心智模型，炼成语言模式金丹', icon: CircleDot },
              { step: '贰', title: '招募道人', desc: '创建 AI 道人，赋予基础性格', icon: Users },
              { step: '叁', title: '服用金丹', desc: '为道人绑定金丹，调配权重与顺序', icon: Sparkles },
              { step: '肆', title: '围炉论道', desc: '与化性后的道人对话，体会丹性涌现', icon: MessageSquare },
            ].map((item, index) => {
              const Icon = item.icon
              return (
                <div key={item.step} className="relative flex flex-col items-center text-center">
                  {/* 连接线（桌面端） */}
                  {index < 3 && (
                    <div className="hidden md:block absolute top-6 left-[60%] w-[80%] h-px bg-gradient-to-r from-bronze-600/40 to-transparent" />
                  )}

                  <div className="w-12 h-12 rounded-full bg-gradient-to-br from-gold-500/20 to-bronze-600/10 border border-gold-500/30 flex items-center justify-center mb-3">
                    <span className="text-sm font-serif font-bold text-gold-400">{item.step}</span>
                  </div>
                  <h4 className="text-sm font-medium text-rice-paper-100 mb-1">{item.title}</h4>
                  <p className="text-xs text-ink-400">{item.desc}</p>
                </div>
              )
            })}
          </div>
        </div>
      </section>
    </div>
  )
}

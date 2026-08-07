/**
 * 顶部导航栏组件 - 道教风格
 * 包含 Logo、导航链接、H5 适配（汉堡菜单 / 底部导航）
 */
import { useState, useEffect } from 'react'
import { Link, useLocation } from 'react-router-dom'
import {
  Flame,
  CircleDot,
  Users,
  MessageSquare,
  Settings,
  Menu,
  X,
  Home,
} from 'lucide-react'

/** 导航项配置 */
const navItems = [
  { path: '/', label: '主殿', icon: Home },
  { path: '/pills', label: '金丹阁', icon: CircleDot },
  { path: '/agents', label: '道人府', icon: Users },
  { path: '/chat', label: '论道', icon: MessageSquare },
  { path: '/settings', label: '设置', icon: Settings },
]

export default function Navbar() {
  const location = useLocation()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [scrolled, setScrolled] = useState(false)

  /** 监听滚动，添加阴影效果 */
  useEffect(() => {
    const handleScroll = () => setScrolled(window.scrollY > 10)
    window.addEventListener('scroll', handleScroll)
    return () => window.removeEventListener('scroll', handleScroll)
  }, [])

  /** 关闭移动端菜单 */
  useEffect(() => {
    setMobileMenuOpen(false)
  }, [location.pathname])

  /** 判断导航项是否激活 */
  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  return (
    <>
      {/* ========== 桌面端顶部导航栏 ========== */}
      <header
        className={`
          fixed top-0 left-0 right-0 z-50
          transition-all duration-300
          ${scrolled ? 'bg-ink-800/95 backdrop-blur-md shadow-lg shadow-black/20' : 'bg-ink-800/80 backdrop-blur-sm'}
          border-b border-bronze-600/20
        `}
      >
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            {/* Logo */}
            <Link to="/" className="flex items-center gap-2.5 group">
              <div className="relative">
                <Flame className="w-7 h-7 text-cinnabar-400 group-hover:text-gold-400 transition-colors" />
                <div className="absolute inset-0 animate-glow opacity-50">
                  <Flame className="w-7 h-7 text-gold-400" />
                </div>
              </div>
              <span className="text-xl font-serif font-bold text-gold-300 tracking-wider">
                炼丹炉
              </span>
            </Link>

            {/* 桌面端导航链接 */}
            <nav className="hidden md:flex items-center gap-1">
              {navItems.map(item => {
                const active = isActive(item.path)
                const Icon = item.icon
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    className={`
                      relative flex items-center gap-1.5 px-3.5 py-2 rounded-lg
                      text-sm font-medium transition-all duration-200
                      ${active
                        ? 'text-gold-300 bg-gold-400/10'
                        : 'text-ink-300 hover:text-gold-300 hover:bg-gold-400/5'
                      }
                    `}
                  >
                    <Icon className="w-4 h-4" />
                    <span>{item.label}</span>
                    {active && (
                      <span className="absolute bottom-0 left-1/2 -translate-x-1/2 w-6 h-0.5 bg-gold-400 rounded-full" />
                    )}
                  </Link>
                )
              })}
            </nav>

            {/* 移动端汉堡菜单按钮 */}
            <button
              className="md:hidden p-2 rounded-lg text-ink-300 hover:text-gold-300 hover:bg-gold-400/10 transition-colors"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              aria-label="切换菜单"
            >
              {mobileMenuOpen ? <X className="w-6 h-6" /> : <Menu className="w-6 h-6" />}
            </button>
          </div>
        </div>

        {/* 移动端下拉菜单 */}
        {mobileMenuOpen && (
          <div className="md:hidden bg-ink-800/98 border-t border-bronze-600/20 animate-fade-in">
            <nav className="px-4 py-3 space-y-1">
              {navItems.map(item => {
                const active = isActive(item.path)
                const Icon = item.icon
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    className={`
                      flex items-center gap-3 px-4 py-3 rounded-lg
                      text-sm font-medium transition-all duration-200
                      ${active
                        ? 'text-gold-300 bg-gold-400/10'
                        : 'text-ink-300 hover:text-gold-300 hover:bg-gold-400/5'
                      }
                    `}
                  >
                    <Icon className="w-5 h-5" />
                    <span>{item.label}</span>
                    {active && <span className="ml-auto w-1.5 h-1.5 rounded-full bg-gold-400" />}
                  </Link>
                )
              })}
            </nav>
          </div>
        )}
      </header>

      {/* ========== 移动端底部导航栏 ========== */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 z-50 bg-ink-800/95 backdrop-blur-md border-t border-bronze-600/20 safe-bottom">
        <div className="flex items-center justify-around h-16">
          {navItems.map(item => {
            const active = isActive(item.path)
            const Icon = item.icon
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`
                  flex flex-col items-center gap-0.5 px-3 py-1.5 rounded-lg
                  transition-all duration-200 min-w-[56px]
                  ${active
                    ? 'text-gold-300'
                    : 'text-ink-400 hover:text-ink-300'
                  }
                `}
              >
                <Icon className={`w-5 h-5 ${active ? 'text-gold-400' : ''}`} />
                <span className="text-[10px] font-medium">{item.label}</span>
                {active && (
                  <span className="absolute bottom-0 w-8 h-0.5 bg-gold-400 rounded-full" />
                )}
              </Link>
            )
          })}
        </div>
      </nav>
    </>
  )
}

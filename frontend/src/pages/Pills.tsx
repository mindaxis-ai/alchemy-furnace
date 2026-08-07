/**
 * 金丹阁页面 - 语言模式技能包管理
 * 金丹列表卡片（道教丹药风格）
 * 关键词搜索 + 内置过滤 + 快捷赠予道人 + 炼制新金丹
 * H5 优化: 单列卡片布局
 */
import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Plus,
  Search,
  CircleDot,
  Loader2,
  FlaskConical,
  X,
} from 'lucide-react'
import { usePill } from '@/contexts/PillContext'
import PillCard from '@/components/PillCard'
import BindAgentModal from '@/components/BindAgentModal'
import Layout from '@/components/Layout'
import { emptySkillSchema } from '@/services/pillService'
import type { Pill } from '@/services/types'

/** 内置过滤选项 */
type BuiltinFilter = 'all' | 'builtin' | 'custom'

export default function Pills() {
  const navigate = useNavigate()
  const { state, fetchPills, addPill } = usePill()
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [builtinFilter, setBuiltinFilter] = useState<BuiltinFilter>('all')
  const [bindingPill, setBindingPill] = useState<Pill | null>(null)
  const [creating, setCreating] = useState(false)

  /** 按搜索条件加载金丹列表 */
  const loadPills = useCallback((keyword: string, filter: BuiltinFilter) => {
    fetchPills({
      keyword: keyword.trim() || undefined,
      is_builtin: filter === 'all' ? undefined : filter === 'builtin',
    })
  }, [fetchPills])

  // 初始化加载
  useEffect(() => {
    loadPills('', 'all')
  }, [loadPills])

  // 搜索防抖
  useEffect(() => {
    const timer = setTimeout(() => loadPills(searchQuery, builtinFilter), 300)
    return () => clearTimeout(timer)
  }, [searchQuery, builtinFilter, loadPills])

  /** 创建金丹（携带空 skill_schema，创建后进入编辑器完善） */
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setCreating(true)
    const pill = await addPill({
      name: name.trim(),
      description: description.trim() || undefined,
      skill_schema: emptySkillSchema(),
      tags: [],
      version: '1.0.0',
    })
    setCreating(false)
    if (pill) {
      setShowCreate(false)
      setName('')
      setDescription('')
      navigate(`/pills/${pill.id}`)
    }
  }

  return (
    <Layout>
      {/* 页面头部 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-3">
            <CircleDot className="w-6 h-6 text-gold-400" />
            <h1 className="page-title">金丹阁</h1>
          </div>
          <p className="page-subtitle">炼制语言模式金丹，塑造道人性情</p>
        </div>

        <button
          onClick={() => setShowCreate(true)}
          className="dao-btn-primary self-start"
        >
          <Plus className="w-4 h-4" />
          炼制新金丹
        </button>
      </div>

      {/* 搜索栏 + 内置过滤 */}
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-400" />
          <input
            type="text"
            placeholder="搜索金丹名称或描述..."
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className="dao-input pl-10"
          />
        </div>
        <select
          value={builtinFilter}
          onChange={e => setBuiltinFilter(e.target.value as BuiltinFilter)}
          className="dao-input sm:w-36"
        >
          <option value="all">全部金丹</option>
          <option value="builtin">系统内置</option>
          <option value="custom">自行炼制</option>
        </select>
      </div>

      {/* 创建金丹弹窗 */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="dao-card w-full max-w-md p-6 animate-fade-in">
            <div className="flex items-center justify-between mb-5">
              <div className="flex items-center gap-2">
                <FlaskConical className="w-5 h-5 text-cinnabar-400" />
                <h2 className="text-lg font-serif font-bold text-gold-300">炼制新金丹</h2>
              </div>
              <button
                onClick={() => setShowCreate(false)}
                className="p-1.5 rounded-lg hover:bg-ink-700 text-ink-400 hover:text-rice-paper-100 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="dao-label">金丹名称 *</label>
                <input
                  type="text"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder="如：文言文金丹"
                  className="dao-input"
                  autoFocus
                  required
                />
              </div>

              <div>
                <label className="dao-label">描述（含触发语、反触发语）</label>
                <textarea
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder="描述这颗金丹赋予的语言模式或人格特质..."
                  className="dao-textarea"
                  rows={3}
                />
                <p className="text-[10px] text-ink-500 mt-1">
                  创建后将进入炼丹房编辑器，完善表达 DNA、心智模型等结构化内容
                </p>
              </div>

              <div className="flex items-center gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreate(false)}
                  className="dao-btn-ghost flex-1"
                >
                  取消
                </button>
                <button
                  type="submit"
                  disabled={!name.trim() || creating}
                  className="dao-btn-primary flex-1 disabled:opacity-50"
                >
                  {creating ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <FlaskConical className="w-4 h-4" />
                  )}
                  开始炼制
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 加载状态 */}
      {state.loading && state.pills.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold-400 animate-spin mb-3" />
          <p className="text-sm text-ink-400">正在搜寻金丹...</p>
        </div>
      )}

      {/* 空状态 */}
      {!state.loading && state.pills.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <CircleDot className="w-12 h-12 text-ink-600 mb-3" />
          <h3 className="text-base font-medium text-ink-400 mb-1">
            {searchQuery || builtinFilter !== 'all' ? '未找到匹配的金丹' : '暂无金丹'}
          </h3>
          <p className="text-sm text-ink-500 mb-4">
            {searchQuery || builtinFilter !== 'all' ? '尝试其他关键词或过滤条件' : '点击上方按钮开始炼制你的第一颗金丹'}
          </p>
          {!searchQuery && builtinFilter === 'all' && (
            <button onClick={() => setShowCreate(true)} className="dao-btn-primary">
              <Plus className="w-4 h-4" />
              炼制新金丹
            </button>
          )}
        </div>
      )}

      {/* 金丹列表 - 桌面端网格，H5 单列 */}
      {state.pills.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {state.pills.map(pill => (
            <PillCard key={pill.id} pill={pill} onBind={setBindingPill} />
          ))}
        </div>
      )}

      {/* 从金丹到道人 - 快捷绑定弹窗 */}
      {bindingPill && (
        <BindAgentModal pill={bindingPill} onClose={() => setBindingPill(null)} />
      )}
    </Layout>
  )
}

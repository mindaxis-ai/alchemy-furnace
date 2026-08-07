/**
 * 道人府页面 - AI Agent 管理
 * 道人卡片列表（道教仙人风格头像占位）
 * 创建道人按钮 + 表单
 * H5 优化: 单列卡片
 */
import { useState, useEffect } from 'react'
import {
  Plus,
  Users,
  Loader2,
  X,
  UserPlus,
  Sparkles,
  Cpu,
  Search,
} from 'lucide-react'
import { useAgent } from '@/contexts/AgentContext'
import AgentCard from '@/components/AgentCard'
import Layout from '@/components/Layout'
import { AVAILABLE_MODELS, DEFAULT_MODEL } from '@/services/models'

export default function Agents() {
  const { state, fetchAgents, addAgent } = useAgent()
  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [personality, setPersonality] = useState('')
  const [modelName, setModelName] = useState(DEFAULT_MODEL)
  const [searchQuery, setSearchQuery] = useState('')

  // 初始化加载
  useEffect(() => {
    fetchAgents()
  }, [fetchAgents])

  /** 创建道人 */
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    const agent = await addAgent({
      name: name.trim(),
      model_name: modelName,
      personality: personality.trim() || undefined,
    })
    if (agent) {
      setShowCreate(false)
      setName('')
      setPersonality('')
      setModelName(DEFAULT_MODEL)
    }
  }

  /** 过滤道人 */
  const filteredAgents = state.agents.filter(agent =>
    agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (agent.personality?.toLowerCase() || '').includes(searchQuery.toLowerCase())
  )

  return (
    <Layout>
      {/* 页面头部 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-3">
            <Users className="w-6 h-6 text-gold-400" />
            <h1 className="page-title">道人府</h1>
          </div>
          <p className="page-subtitle">管理你的 AI 道人</p>
        </div>

        <button
          onClick={() => setShowCreate(true)}
          className="dao-btn-primary self-start"
        >
          <Plus className="w-4 h-4" />
          招募道人
        </button>
      </div>

      {/* 搜索栏 */}
      <div className="relative mb-6">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-ink-400" />
        <input
          type="text"
          placeholder="搜索道人..."
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          className="dao-input pl-10"
        />
      </div>

      {/* 创建道人弹窗 */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="dao-card w-full max-w-md p-6 animate-fade-in max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-5">
              <div className="flex items-center gap-2">
                <UserPlus className="w-5 h-5 text-jade-400" />
                <h2 className="text-lg font-serif font-bold text-gold-300">招募道人</h2>
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
                <label className="dao-label">道号 *</label>
                <input
                  type="text"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder="如：太虚真人"
                  className="dao-input"
                  autoFocus
                  required
                />
              </div>

              <div>
                <label className="dao-label">基础性格 / 系统提示词</label>
                <textarea
                  value={personality}
                  onChange={e => setPersonality(e.target.value)}
                  placeholder="描述这位道人的性格特点和语言风格..."
                  className="dao-textarea"
                  rows={4}
                />
                <p className="text-[10px] text-ink-500 mt-1">
                  基础性格将与已服用金丹的语言模式合成，决定道人的表达风格
                </p>
              </div>

              <div>
                <label className="dao-label flex items-center gap-1.5">
                  <Cpu className="w-3.5 h-3.5" />
                  选择模型
                </label>
                <select
                  value={modelName}
                  onChange={e => setModelName(e.target.value)}
                  className="dao-input"
                >
                  {AVAILABLE_MODELS.map(model => (
                    <option key={model.id} value={model.id}>
                      {model.name} - {model.description}
                    </option>
                  ))}
                </select>
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
                  disabled={!name.trim() || state.loading}
                  className="dao-btn-primary flex-1 disabled:opacity-50"
                >
                  {state.loading ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <Sparkles className="w-4 h-4" />
                  )}
                  招募
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 加载状态 */}
      {state.loading && state.agents.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16">
          <Loader2 className="w-8 h-8 text-gold-400 animate-spin mb-3" />
          <p className="text-sm text-ink-400">正在寻访道人...</p>
        </div>
      )}

      {/* 空状态 */}
      {!state.loading && filteredAgents.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Users className="w-12 h-12 text-ink-600 mb-3" />
          <h3 className="text-base font-medium text-ink-400 mb-1">
            {searchQuery ? '未找到匹配的道人' : '暂无道人'}
          </h3>
          <p className="text-sm text-ink-500 mb-4">
            {searchQuery ? '尝试其他关键词' : '点击上方按钮招募你的第一位道人'}
          </p>
          {!searchQuery && (
            <button onClick={() => setShowCreate(true)} className="dao-btn-primary">
              <Plus className="w-4 h-4" />
              招募道人
            </button>
          )}
        </div>
      )}

      {/* 道人列表 */}
      {filteredAgents.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredAgents.map(agent => (
            <AgentCard
              key={agent.id}
              agent={agent}
            />
          ))}
        </div>
      )}
    </Layout>
  )
}

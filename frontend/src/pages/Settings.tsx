/**
 * 设置页 - 系统配置
 * API Key 配置、模型选择、关于信息
 * 响应式布局
 */
import { useState } from 'react'
import {
  Settings2,
  Key,
  Globe,
  Cpu,
  Info,
  Flame,
  Save,
  Check,
  ExternalLink,
  Heart,
} from 'lucide-react'
import Layout from '@/components/Layout'
import { AVAILABLE_MODELS, DEFAULT_MODEL } from '@/services/models'

export default function Settings() {
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('https://api.openai.com/v1')
  const [defaultModel, setDefaultModel] = useState(DEFAULT_MODEL)
  const [saved, setSaved] = useState(false)

  /** 保存设置 */
  const handleSave = () => {
    // 演示模式：仅显示保存成功
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  return (
    <Layout>
      {/* 页面头部 */}
      <div className="flex items-center gap-3 mb-6">
        <Settings2 className="w-6 h-6 text-gold-400" />
        <div>
          <h1 className="page-title">设置</h1>
          <p className="page-subtitle">配置系统参数</p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* 左侧：主要设置 */}
        <div className="lg:col-span-2 space-y-6">
          {/* API 配置 */}
          <section className="dao-card p-5 md:p-6">
            <div className="flex items-center gap-2 mb-5">
              <Key className="w-5 h-5 text-gold-400" />
              <h2 className="text-lg font-serif font-bold text-gold-300">API 配置</h2>
            </div>

            <div className="space-y-4">
              <div>
                <label className="dao-label flex items-center gap-1.5">
                  <Key className="w-3.5 h-3.5" />
                  API Key
                </label>
                <input
                  type="password"
                  value={apiKey}
                  onChange={e => setApiKey(e.target.value)}
                  placeholder="sk-xxxxxxxxxxxxxxxxxxxxxxxx"
                  className="dao-input"
                />
                <p className="text-[10px] text-ink-500 mt-1">
                  你的 OpenAI / DeepSeek / 通义千问 API Key
                </p>
              </div>

              <div>
                <label className="dao-label flex items-center gap-1.5">
                  <Globe className="w-3.5 h-3.5" />
                  API Base URL
                </label>
                <input
                  type="text"
                  value={baseUrl}
                  onChange={e => setBaseUrl(e.target.value)}
                  placeholder="https://api.openai.com/v1"
                  className="dao-input"
                />
              </div>

              <div>
                <label className="dao-label flex items-center gap-1.5">
                  <Cpu className="w-3.5 h-3.5" />
                  默认模型
                </label>
                <select
                  value={defaultModel}
                  onChange={e => setDefaultModel(e.target.value)}
                  className="dao-input"
                >
                  {AVAILABLE_MODELS.map(model => (
                    <option key={model.id} value={model.id}>
                      {model.name} ({model.provider})
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </section>

          {/* 模型列表 */}
          <section className="dao-card p-5 md:p-6">
            <div className="flex items-center gap-2 mb-5">
              <Cpu className="w-5 h-5 text-jade-400" />
              <h2 className="text-lg font-serif font-bold text-gold-300">可用模型</h2>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {AVAILABLE_MODELS.map(model => (
                <div
                  key={model.id}
                  className="flex items-center gap-3 p-3 rounded-lg bg-ink-800/50 border border-bronze-600/20"
                >
                  <div className="w-9 h-9 rounded-lg bg-jade-500/15 flex items-center justify-center flex-shrink-0">
                    <Cpu className="w-4.5 h-4.5 text-jade-400" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-rice-paper-100">{model.name}</p>
                    <p className="text-[10px] text-ink-400">{model.description}</p>
                  </div>
                  <span className={`
                    text-[10px] px-2 py-0.5 rounded-full border
                    ${model.provider === 'openai' ? 'bg-blue-500/10 text-blue-400 border-blue-500/20' :
                      model.provider === 'deepseek' ? 'bg-purple-500/10 text-purple-400 border-purple-500/20' :
                        'bg-orange-500/10 text-orange-400 border-orange-500/20'}
                  `}>
                    {model.provider}
                  </span>
                </div>
              ))}
            </div>
          </section>

          {/* 保存按钮 */}
          <div className="flex items-center justify-end gap-3">
            {saved && (
              <span className="flex items-center gap-1 text-sm text-jade-400 animate-fade-in">
                <Check className="w-4 h-4" />
                已保存
              </span>
            )}
            <button onClick={handleSave} className="dao-btn-primary">
              <Save className="w-4 h-4" />
              保存设置
            </button>
          </div>
        </div>

        {/* 右侧：关于信息 */}
        <div className="space-y-6">
          <section className="dao-card p-5">
            <div className="flex items-center gap-2 mb-4">
              <Info className="w-5 h-5 text-gold-400" />
              <h2 className="text-base font-serif font-bold text-gold-300">关于炼丹炉</h2>
            </div>

            <div className="flex flex-col items-center text-center py-4">
              <div className="w-16 h-16 rounded-2xl bg-gradient-to-b from-cinnabar-500/20 to-ink-800/80 border-2 border-gold-500/30 flex items-center justify-center mb-3 glow-gold">
                <Flame className="w-8 h-8 text-cinnabar-400" />
              </div>
              <h3 className="text-lg font-serif font-bold text-gold-300 mb-1">炼丹炉</h3>
              <p className="text-xs text-ink-400 mb-4">v1.0.0</p>

              <p className="text-sm text-ink-400 leading-relaxed mb-4">
                以道教炼丹文化为设计灵感的金丹化性系统。
                金丹即语言模式技能包，道人即 AI Agent，服丹化性，围炉论道。
              </p>

              <div className="dao-divider text-[10px] w-full mb-4">
                <Heart className="w-3 h-3" />
              </div>

              <div className="space-y-2 w-full text-left">
                <a
                  href="https://github.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-2 p-2.5 rounded-lg bg-ink-800/50 hover:bg-gold-400/5 border border-bronze-600/20 hover:border-gold-400/30 transition-all text-sm"
                >
                  <ExternalLink className="w-4 h-4 text-gold-400" />
                  <span className="text-rice-paper-200">GitHub 仓库</span>
                </a>
              </div>
            </div>
          </section>

          {/* 技术栈 */}
          <section className="dao-card p-5">
            <h3 className="text-sm font-medium text-gold-300 mb-3">技术栈</h3>
            <div className="flex flex-wrap gap-2">
              {['React 18', 'TypeScript', 'Tailwind CSS', 'Vite', 'Go API', 'Python 语言引擎', 'PostgreSQL'].map(tech => (
                <span
                  key={tech}
                  className="px-2.5 py-1 text-[11px] rounded-full bg-jade-500/10 text-jade-400 border border-jade-500/20"
                >
                  {tech}
                </span>
              ))}
            </div>
          </section>
        </div>
      </div>
    </Layout>
  )
}

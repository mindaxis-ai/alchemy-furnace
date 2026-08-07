/**
 * 可选 LLM 模型列表
 * 前端静态配置，用于道人创建/编辑时的模型选择
 */
import type { ModelConfig } from './types'

export const AVAILABLE_MODELS: ModelConfig[] = [
  { id: 'gpt-4o', name: 'GPT-4o', description: 'OpenAI 旗舰模型', provider: 'openai' },
  { id: 'gpt-4o-mini', name: 'GPT-4o Mini', description: '轻量快速', provider: 'openai' },
  { id: 'deepseek-chat', name: 'DeepSeek-V3', description: '深度求索', provider: 'deepseek' },
  { id: 'deepseek-reasoner', name: 'DeepSeek-R1', description: '推理增强', provider: 'deepseek' },
  { id: 'qwen-turbo', name: '通义千问 Turbo', description: '阿里通义', provider: 'aliyun' },
  { id: 'qwen-plus', name: '通义千问 Plus', description: '增强版', provider: 'aliyun' },
]

/** 默认模型 */
export const DEFAULT_MODEL = AVAILABLE_MODELS[0].id

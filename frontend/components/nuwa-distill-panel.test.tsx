import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { NuwaDistillPanel } from '@/components/nuwa-distill-panel'
import { ApiError } from '@/services/api'
import type { DistillationDraft } from '@/services/types'

const distillNuwa = vi.hoisted(() => vi.fn())

vi.mock('@/services/distillationService', () => ({
  distillNuwa,
}))

// key 透传：面板断言只关心键/角色与草稿内容，不依赖具体语言
vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
  useLocale: () => 'zh-CN',
}))

const nuwaDraft: DistillationDraft = {
  name: '女娲草稿',
  description: '由女娲蒸馏出的候选',
  persona_summary: '冷静克制的史官',
  tags: ['神话', '蒸馏'],
  skill_schema: { identity_card: '史官' },
  sources: [
    { title: '史记', url: 'https://example.com/shiji', dimension: 'tone' },
    { title: '资治通鉴', url: 'https://example.com/zztj', dimension: 'fact' },
  ],
  model: 'gpt-5',
}

/** 填满主题与目标并触发蒸馏 */
async function runDistill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByPlaceholderText('subjectPlaceholder'), '保罗·格雷厄姆')
  await user.type(screen.getByPlaceholderText('briefPlaceholder'), '提取他的判断方式')
  await user.click(screen.getByRole('button', { name: 'start' }))
}

describe('NuwaDistillPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    cleanup()
  })

  it('蒸馏完成后只显示来源与候选预览,不自动应用', async () => {
    distillNuwa.mockResolvedValue(nuwaDraft)
    const onApply = vi.fn()
    const user = userEvent.setup()
    render(<NuwaDistillPanel onApply={onApply} />)

    await runDistill(user)

    // 预览出现:草稿名称、来源标题可见
    expect(await screen.findByText('女娲草稿')).toBeInTheDocument()
    expect(screen.getByText('史记')).toBeInTheDocument()
    // 关键:未经确认不得自动应用
    expect(onApply).not.toHaveBeenCalled()
  })

  it('只有用户显式确认「应用」后才写入表单', async () => {
    distillNuwa.mockResolvedValue(nuwaDraft)
    const onApply = vi.fn()
    const user = userEvent.setup()
    render(<NuwaDistillPanel onApply={onApply} />)

    await runDistill(user)
    await screen.findByText('女娲草稿')
    expect(onApply).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'apply' }))
    expect(onApply).toHaveBeenCalledTimes(1)
    expect(onApply).toHaveBeenCalledWith(nuwaDraft)
  })

  it('用户可丢弃候选草稿,丢弃后永不应用', async () => {
    distillNuwa.mockResolvedValue(nuwaDraft)
    const onApply = vi.fn()
    const user = userEvent.setup()
    render(<NuwaDistillPanel onApply={onApply} />)

    await runDistill(user)
    await screen.findByText('女娲草稿')

    await user.click(screen.getByRole('button', { name: 'discard' }))
    expect(screen.queryByText('女娲草稿')).not.toBeInTheDocument()
    expect(onApply).not.toHaveBeenCalled()
  })

  it('蒸馏失败显示错误,不产生候选也不应用', async () => {
    distillNuwa.mockRejectedValue(new ApiError('服务器内部错误', 500))
    const onApply = vi.fn()
    const user = userEvent.setup()
    render(<NuwaDistillPanel onApply={onApply} />)

    await runDistill(user)

    expect(await screen.findByText('服务器内部错误')).toBeInTheDocument()
    expect(screen.queryByText('女娲草稿')).not.toBeInTheDocument()
    expect(onApply).not.toHaveBeenCalled()
  })

  it('始终走正式蒸馏链路,以用户输入调用 distillNuwa(无 Mock fallback)', async () => {
    distillNuwa.mockResolvedValue(nuwaDraft)
    const onApply = vi.fn()
    const user = userEvent.setup()
    render(<NuwaDistillPanel onApply={onApply} />)

    await runDistill(user)
    await screen.findByText('女娲草稿')

    expect(distillNuwa).toHaveBeenCalledTimes(1)
    expect(distillNuwa).toHaveBeenCalledWith({
      subject: '保罗·格雷厄姆',
      brief: '提取他的判断方式',
      locale: 'zh-CN',
    })
  })
})

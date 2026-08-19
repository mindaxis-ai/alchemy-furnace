import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, expectTypeOf, it, vi } from 'vitest'
import { ActionFeedback, type ActionFeedbackProps } from './action-feedback'

describe('ActionFeedback', () => {
  it('announces submission progress', () => {
    render(<ActionFeedback status="submitting" message="正在提交" />)

    expect(screen.getByRole('status')).toHaveTextContent('正在提交')
  })

  it('keeps the error visible and retries the failed action', async () => {
    const retry = vi.fn()
    render(
      <ActionFeedback
        status="error"
        message="模型未配置"
        retryLabel="Retry"
        onRetry={retry}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent('模型未配置')
    await userEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('requires localized retry details for error feedback', () => {
    expectTypeOf<Extract<ActionFeedbackProps, { status: 'error' }>>().toEqualTypeOf<{
      status: 'error'
      message: string
      retryLabel: string
      onRetry: () => void
    }>()
  })
})

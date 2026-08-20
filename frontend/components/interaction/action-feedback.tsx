export type ActionFeedbackStatus = 'submitting' | 'error'

export type ActionFeedbackProps =
  | {
      status: 'submitting'
      message: string
    }
  | {
      status: 'error'
      message: string
      onRetry: () => void
      retryLabel: string
    }

export function ActionFeedback(props: ActionFeedbackProps) {
  if (props.status === 'submitting') {
    return (
      <p role="status" aria-live="polite" className="break-words text-sm text-sage">
        {props.message}
      </p>
    )
  }

  return (
    <div role="alert" className="flex min-w-0 flex-wrap items-center gap-2 text-sm text-primary">
      <span className="min-w-0 flex-1 break-words">{props.message}</span>
      <button type="button" onClick={props.onRetry} className="dao-btn-secondary shrink-0 text-xs">
        {props.retryLabel}
      </button>
    </div>
  )
}

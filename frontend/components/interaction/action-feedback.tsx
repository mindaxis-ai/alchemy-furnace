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
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {props.message}
      </p>
    )
  }

  return (
    <div role="alert" className="flex items-center gap-3 text-sm text-red-500">
      <span>{props.message}</span>
      <button type="button" onClick={props.onRetry} className="dao-btn-secondary text-xs">
        {props.retryLabel}
      </button>
    </div>
  )
}

export type ActionFeedbackStatus = 'submitting' | 'error'

interface ActionFeedbackProps {
  status: ActionFeedbackStatus
  message: string
  onRetry?: () => void
  retryLabel?: string
}

export function ActionFeedback({
  status,
  message,
  onRetry,
  retryLabel = 'Retry',
}: ActionFeedbackProps) {
  if (status === 'submitting') {
    return (
      <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
        {message}
      </p>
    )
  }

  return (
    <div role="alert" className="flex items-center gap-3 text-sm text-red-500">
      <span>{message}</span>
      {onRetry && (
        <button type="button" onClick={onRetry} className="dao-btn-secondary text-xs">
          {retryLabel}
        </button>
      )}
    </div>
  )
}

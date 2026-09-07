import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AvatarSourceInput } from '@/components/avatar-source-input'

describe('AvatarSourceInput', () => {
  afterEach(cleanup)

  it('returns a local image as a data URI', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(
      <AvatarSourceInput
        inputId="avatar"
        name="太上老君"
        value=""
        onChange={onChange}
        onError={() => undefined}
        label="头像 URL"
        linkPlaceholder="https://example.com/avatar.png"
        uploadLabel="本地上传"
        previewAlt="头像预览"
      />,
    )

    await user.upload(
      screen.getByLabelText('本地上传 file'),
      new File(['avatar'], 'avatar.png', { type: 'image/png' }),
    )

    await waitFor(() => expect(onChange).toHaveBeenCalledWith(expect.stringMatching(/^data:image\/png;base64,/)))
  })

  it('rejects unsupported local file types', async () => {
    const onChange = vi.fn()
    const onError = vi.fn()
    render(
      <AvatarSourceInput
        inputId="avatar"
        name="太上老君"
        value=""
        onChange={onChange}
        onError={onError}
        label="头像 URL"
        linkPlaceholder="https://example.com/avatar.png"
        uploadLabel="本地上传"
        previewAlt="头像预览"
      />,
    )

    fireEvent.change(screen.getByLabelText('本地上传 file'), {
      target: { files: [new File(['not an image'], 'avatar.svg', { type: 'image/svg+xml' })] },
    })

    expect(onError).toHaveBeenCalledWith('invalid')
    expect(onChange).not.toHaveBeenCalled()
  })
})

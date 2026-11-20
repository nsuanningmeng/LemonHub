/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { KeyboardEvent } from 'react'
import { useForm } from 'react-hook-form'
import { expect, test, vi } from 'vitest'

import { TagInput } from '@/components/tag-input'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

function FormFixture() {
  const form = useForm<{ tags: string[] }>({ defaultValues: { tags: [] } })
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(() => undefined)}>
        <FormField
          control={form.control}
          name='tags'
          rules={{ validate: (value) => value.length > 0 || 'Choose a tag' }}
          render={({ field }) => (
            <FormItem>
              <FormLabel>Models</FormLabel>
              <FormControl>
                <TagInput
                  value={field.value}
                  onChange={field.onChange}
                  onBlur={field.onBlur}
                  ref={field.ref}
                  name={field.name}
                />
              </FormControl>
              <FormDescription>Select models to enable</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <button type='submit'>Save</button>
        <button type='button' onClick={() => form.setFocus('tags')}>
          Focus models
        </button>
        <output aria-label='Touched'>
          {String(!!form.formState.touchedFields.tags)}
        </output>
      </form>
    </Form>
  )
}

test.each([
  { isComposing: true, keyCode: 13 },
  { isComposing: false, keyCode: 229 },
])(
  'confirming an IME candidate ($isComposing/$keyCode) leaves the draft uncommitted',
  (event) => {
    const onChange = vi.fn()
    render(
      <TagInput
        value={[]}
        onChange={onChange}
        placeholder='Models'
        aria-label='Models'
      />
    )
    const input = screen.getByPlaceholderText('Models')
    fireEvent.change(input, { target: { value: 'zhong' } })
    fireEvent.keyDown(input, { key: 'Enter', ...event })
    expect(onChange).not.toHaveBeenCalled()
    expect(input).toHaveValue('zhong')
    fireEvent.change(input, { target: { value: '中文' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith(['中文'])
  }
)

test('form labels, descriptions, validation and focus reach the input', async () => {
  const user = userEvent.setup()
  render(<FormFixture />)
  const input = screen.getByRole('textbox', { name: 'Models' })
  expect(input).toHaveAccessibleDescription('Select models to enable')
  expect(input).toHaveAttribute('aria-invalid', 'false')
  await user.click(screen.getByRole('button', { name: 'Focus models' }))
  expect(input).toHaveFocus()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
  expect(input).toHaveAccessibleDescription(
    'Select models to enable Choose a tag'
  )
  await waitFor(() => expect(input).toHaveFocus())
  expect(screen.getByLabelText('Touched')).toHaveTextContent('true')
})

test('a caller can prevent Enter without losing its keyboard callback', () => {
  const onChange = vi.fn()
  const onKeyDown = vi.fn((event: KeyboardEvent<HTMLInputElement>) =>
    event.preventDefault()
  )
  render(
    <TagInput
      value={[]}
      onChange={onChange}
      onKeyDown={onKeyDown}
      aria-label='Models'
    />
  )
  const input = screen.getByRole('textbox', { name: 'Models' })
  fireEvent.change(input, { target: { value: 'model-one' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(onKeyDown).toHaveBeenCalledOnce()
  expect(onChange).not.toHaveBeenCalled()
  expect(input).toHaveValue('model-one')
})

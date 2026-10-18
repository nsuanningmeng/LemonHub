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
import { expect, test } from 'vitest'

import { channelSchema } from '../../types'
import { CHANNEL_TYPE_ADVANCED_CUSTOM } from '../advanced-custom'
import {
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
} from '../channel-form'

function hiddenChannelDefaults() {
  return transformChannelToFormDefaults(
    channelSchema.parse({
      id: 42,
      name: 'Hidden custom channel',
      type: CHANNEL_TYPE_ADVANCED_CUSTOM,
      key: '',
      status: 1,
      created_time: 1,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'model',
      group: 'default',
      base_url: null,
      base_url_redacted: true,
      proxy_configured: true,
      advanced_custom_configured: true,
      setting: '{}',
      settings: '{"upstream_model_update_check_enabled":true}',
    })
  )
}

test('an unrelated update preserves redacted custom routes and does not expose local preservation flags', () => {
  const form = channelFormSchema.parse({
    ...hiddenChannelDefaults(),
    name: 'Renamed',
  })
  const payload = transformFormDataToUpdatePayload(form, 42)
  expect(payload.name).toBe('Renamed')
  expect(payload).not.toHaveProperty('base_url')
  expect(JSON.parse(payload.setting ?? '{}')).not.toHaveProperty('proxy')
  expect(JSON.parse(payload.settings ?? '{}')).not.toHaveProperty(
    'advanced_custom'
  )
  expect(
    Object.keys(payload).some((key) => key.startsWith('preserve_hidden_'))
  ).toBe(false)
})

test('explicit custom route replacement is serialized while its hidden base URL remains preserved', () => {
  const replacement = {
    advanced_routes: [
      {
        incoming_path: '/v1/chat/completions',
        upstream_path: '/provider/chat',
        converter: 'none',
        auth: {
          type: 'header',
          name: 'x-api-key',
          value: 'replacement-secret',
        },
      },
    ],
  }
  const form = channelFormSchema.parse({
    ...hiddenChannelDefaults(),
    preserve_hidden_advanced_custom: false,
    advanced_custom: JSON.stringify(replacement),
    upstream_model_update_check_enabled: false,
  })
  const payload = transformFormDataToUpdatePayload(form, 42)
  expect(JSON.parse(payload.settings ?? '{}').advanced_custom).toEqual(
    replacement
  )
  expect(payload).not.toHaveProperty('base_url')
})

test('an explicit invalid replacement still fails validation instead of retaining the hidden configuration', () => {
  const result = channelFormSchema.safeParse({
    ...hiddenChannelDefaults(),
    preserve_hidden_advanced_custom: false,
    advanced_custom: '',
  })
  expect(result.success).toBe(false)
  if (!result.success) {
    expect(
      result.error.issues.some((issue) => issue.path[0] === 'advanced_custom')
    ).toBe(true)
  }
})

test('an explicitly cleared optional base URL is sent as an empty string', () => {
  const form = channelFormSchema.parse({
    ...hiddenChannelDefaults(),
    type: 1,
    preserve_hidden_base_url: false,
    base_url: '',
  })
  expect(transformFormDataToUpdatePayload(form, 42)).toHaveProperty(
    'base_url',
    ''
  )
})

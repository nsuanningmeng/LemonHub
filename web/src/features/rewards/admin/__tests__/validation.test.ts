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
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  campaignFormToInput,
  getCampaignFormSchema,
  type CampaignFormValues,
} from '../lib/campaign-form'

const originalConfig = useSystemConfigStore.getState().config
const campaign: CampaignFormValues = {
  title: 'October loyalty reward',
  description: 'Thank you for creating with us.',
  type: 'random',
  minimum: '1',
  maximum: '5',
  audience: 'either',
  expires_in_days: 7,
  start: '2026-10-01T00:00',
  end: '2026-10-08T00:00',
}

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 500000 },
  })
})

afterEach(() => {
  useSystemConfigStore.getState().setConfig(originalConfig)
  localStorage.clear()
})

describe('reward campaign validation', () => {
  test.each([0, -1, 1.5, 3651, Number.NaN])(
    'rejects invalid reward validity of %s days',
    (expires_in_days) => {
      const result = getCampaignFormSchema(i18next.t).safeParse({
        ...campaign,
        expires_in_days,
        continuous: true,
      })

      expect(result.success).toBe(false)
    }
  )

  test.each([1, 3650])(
    'accepts %s valid days at the boundary and preserves the value in the request',
    (expires_in_days) => {
      const parsed = getCampaignFormSchema(i18next.t).parse({
        ...campaign,
        expires_in_days,
        continuous: true,
      })

      expect(campaignFormToInput(parsed, 'validity-boundary')).toMatchObject({
        expires_in_days,
        continuous: true,
      })
    }
  )

  test('accepts users with both successful top-ups and API calls as an audience', () => {
    const parsed = getCampaignFormSchema(i18next.t).parse({
      ...campaign,
      audience: 'both',
    })

    expect(campaignFormToInput(parsed, 'both-activities')).toMatchObject({
      audience: 'both',
    })
  })

  test('future activity dates retain their source-record period and opt into continuous delivery', () => {
    const parsed = getCampaignFormSchema(i18next.t).parse({
      ...campaign,
      start: '2099-11-01T00:00',
      end: '2099-11-08T00:00',
    })

    const input = campaignFormToInput(parsed, 'future-activity')

    expect(input).toMatchObject({
      start_time: new Date('2099-11-01T00:00').getTime() / 1000,
      end_time: new Date('2099-11-08T00:00').getTime() / 1000,
      expires_in_days: 7,
      continuous: true,
    })
    expect(input).not.toHaveProperty('scheduled_at')
  })

  test.each(['', '0', '-1', '18014398510'])(
    'rejects an invalid or unsafe reward amount %j',
    (minimum) => {
      const result = getCampaignFormSchema(i18next.t).safeParse({
        ...campaign,
        minimum,
      })

      expect(result.success).toBe(false)
    }
  )

  test('equal random reward bounds are valid and remain equal in the request', () => {
    const parsed = getCampaignFormSchema(i18next.t).parse({
      ...campaign,
      maximum: '1',
    })

    expect(campaignFormToInput(parsed, 'same-bounds')).toMatchObject({
      type: 'random',
      min_quota: 500000,
      max_quota: 500000,
    })
  })

  test.each(['2026-10-01T00:00', '2026-09-30T00:00', 'invalid'])(
    'rejects an end time %j that cannot define a positive interval',
    (end) => {
      const result = getCampaignFormSchema(i18next.t).safeParse({
        ...campaign,
        end,
      })

      expect(result.success).toBe(false)
    }
  )

  test('USD input uses quota conversion without applying the display exchange rate', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7,
        quotaPerUnit: 500000,
      },
    })
    const parsed = getCampaignFormSchema(i18next.t).parse({
      ...campaign,
      type: 'fixed',
      minimum: '2.5',
      maximum: '',
    })

    expect(campaignFormToInput(parsed, 'usd-reward')).toMatchObject({
      type: 'fixed',
      min_quota: 1250000,
      max_quota: 1250000,
    })
  })
})

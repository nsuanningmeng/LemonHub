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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { RoutingReliabilitySection } from '../routing-reliability-section'

const autoBanIntervalDescription =
  'How frequently the system tests channels with auto-disable enabled'
const allChannelsIntervalDescription =
  'How frequently the system tests all channels'

describe('RoutingReliabilitySection', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      [allChannelsIntervalDescription]: allChannelsIntervalDescription,
      [autoBanIntervalDescription]: autoBanIntervalDescription,
    })
  })

  test('describes the auto-ban-only interval with its restricted channel scope', () => {
    const queryClient = new QueryClient()

    render(
      <QueryClientProvider client={queryClient}>
        <RoutingReliabilitySection
          defaultValues={{
            RetryTimes: 0,
            ChannelDisableThreshold: '',
            AutomaticDisableChannelEnabled: true,
            AutomaticEnableChannelEnabled: true,
            AutomaticDisableKeywords: '',
            AutomaticDisableStatusCodes: '',
            AutomaticRetryStatusCodes: '',
            ErrorOverrideGlobalEnabled: false,
            ErrorOverrideGlobalMessage: '',
            ErrorOverrideKeywords: '',
            'monitor_setting.auto_test_channel_enabled': true,
            'monitor_setting.auto_test_channel_minutes': 10,
            'monitor_setting.channel_test_concurrency': 1,
            'monitor_setting.channel_test_mode': 'auto_ban_only',
          }}
        />
      </QueryClientProvider>
    )

    expect(screen.getByText(autoBanIntervalDescription)).toBeInTheDocument()
    expect(
      screen.queryByText(allChannelsIntervalDescription)
    ).not.toBeInTheDocument()

    queryClient.clear()
  })
})

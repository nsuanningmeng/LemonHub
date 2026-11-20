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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isCancel } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'

import { updateSystemOption } from '../api'
import { markSettingsErrorReported } from '../lib/settings-action'
import type { UpdateOptionRequest } from '../types'

// Configuration keys that require status refresh
const STATUS_RELATED_KEYS = new Set([
  'SystemName',
  'Logo',
  'Footer',
  'ServerAddress',
  'DefaultCollapseSidebar',
  'DemoSiteEnabled',
  'SelfUseModeEnabled',
  'general_setting.docs_link',
  'QuotaRemindEnabled',
  'QuotaRemindThreshold',
  'HeaderNavModules',
  'SidebarModulesAdmin',
  'Notice',
  'LogConsumeEnabled',
  'QuotaPerUnit',
  'USDExchangeRate',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
  'general_setting.quota_display_type',
  'general_setting.custom_currency_symbol',
  'general_setting.custom_currency_exchange_rate',
  // SEO meta tags are injected from /api/status (see main.tsx applySeo)
  'SeoDescription',
  'SeoKeywords',
  // Model performance success-rate display config is read via /api/status
  'perf_metrics_setting.success_rate_green_threshold',
  'perf_metrics_setting.success_rate_yellow_threshold',
  'perf_metrics_setting.no_data_as_full',
  'oidc.display_name',
  // Login widgets must use the same captcha provider and public credentials
  // as the backend immediately after a settings change.
  'TurnstileCheckEnabled',
  'CaptchaProvider',
  'TurnstileSiteKey',
  'GeetestCaptchaId',
  'TencentCaptchaAppId',
  'RegisterEnabled',
  'PasswordLoginEnabled',
  'PasswordRegisterEnabled',
  'EmailVerificationEnabled',
  'GitHubOAuthEnabled',
  'GitHubClientId',
  'discord.enabled',
  'discord.client_id',
  'oidc.enabled',
  'oidc.client_id',
  'oidc.authorization_endpoint',
  'LinuxDOOAuthEnabled',
  'LinuxDOClientId',
  'LinuxDOMinimumTrustLevel',
  'TelegramOAuthEnabled',
  'TelegramBotName',
  'WeChatAuthEnabled',
  'WeChatAccountQRCodeImageURL',
  'passkey.enabled',
  'passkey.rp_display_name',
  'passkey.rp_id',
  'passkey.origins',
  'passkey.allow_insecure_origin',
  'passkey.user_verification',
  'passkey.attachment_preference',
  'legal.user_agreement',
  'legal.privacy_policy',
  'Chats',
  'DataExportEnabled',
  'DataExportDefaultTime',
  'DrawingEnabled',
  'TaskEnabled',
  'MjNotifyEnabled',
  'BatchUpdateEnabled',
  'DefaultUseAutoGroup',
  'console_setting.api_info_enabled',
  'console_setting.api_info',
  'console_setting.announcements_enabled',
  'console_setting.announcements',
  'console_setting.faq_enabled',
  'console_setting.faq',
  'console_setting.uptime_kuma_enabled',
])

const CONTENT_QUERY_KEYS: Record<string, string[]> = {
  Notice: ['notice'],
  HomePageContent: ['home-page-content'],
  About: ['about-content'],
  Contact: ['contact-content'],
  'legal.user_agreement': ['user-agreement'],
  'legal.privacy_policy': ['privacy-policy'],
  'console_setting.uptime_kuma_groups': ['uptime-status'],
}

export function useUpdateOption() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (request: UpdateOptionRequest) => updateSystemOption(request),
    onSuccess: async (data, variables) => {
      if (data.success) {
        // Always refresh system-options
        const refreshes = [
          queryClient.invalidateQueries({ queryKey: ['system-options'] }),
        ]

        // If updating frontend-display-related config, also refresh status
        if (STATUS_RELATED_KEYS.has(variables.key)) {
          try {
            window.localStorage.removeItem('status')
          } catch {
            /* empty */
          }
          refreshes.push(
            queryClient.invalidateQueries({ queryKey: ['status'] })
          )
        }

        const contentQueryKey = CONTENT_QUERY_KEYS[variables.key]
        if (contentQueryKey) {
          if (variables.key === 'HomePageContent') {
            try {
              window.localStorage.removeItem('home_page_content')
            } catch {
              /* empty */
            }
          }
          refreshes.push(
            queryClient.invalidateQueries({ queryKey: contentQueryKey })
          )
        }
        await Promise.all(refreshes)
        toast.success(i18next.t('Setting updated successfully'))
      } else {
        toast.error(data.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      if (!isCancel(error)) {
        toast.error(error.message || i18next.t('Failed to update setting'))
      }
      markSettingsErrorReported(error)
    },
  })
}

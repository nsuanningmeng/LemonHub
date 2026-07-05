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
import { SystemBehaviorSection } from '../general/system-behavior-section'
import { EmailSettingsSection } from '../integrations/email-settings-section'
import { MonitoringSettingsSection } from '../integrations/monitoring-settings-section'
import { WorkerSettingsSection } from '../integrations/worker-settings-section'
import { LogSettingsSection } from '../maintenance/log-settings-section'
import { PerformanceSection } from '../maintenance/performance-section'
import { UpdateCheckerSection } from '../maintenance/update-checker-section'
import type { OperationsSettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'
import { EmailPromotionSection } from './email-promotion-section'
import { EmailSuppressionSection } from './email-suppression-section'
import { MarketingSmtpSection } from './marketing-smtp-section'
import { TicketSection } from './ticket-section'

const OPERATIONS_SECTIONS = [
  {
    id: 'behavior',
    titleKey: 'System Behavior',
    build: (settings: OperationsSettings) => (
      <SystemBehaviorSection
        defaultValues={{
          DefaultCollapseSidebar: settings.DefaultCollapseSidebar,
          DemoSiteEnabled: settings.DemoSiteEnabled,
          SelfUseModeEnabled: settings.SelfUseModeEnabled,
        }}
      />
    ),
  },
  {
    id: 'alerts',
    titleKey: 'Monitoring & Alerts',
    build: (settings: OperationsSettings) => (
      <MonitoringSettingsSection
        defaultValues={{
          QuotaRemindThreshold: settings.QuotaRemindThreshold,
          'perf_metrics_setting.enabled':
            settings['perf_metrics_setting.enabled'] ?? true,
          'perf_metrics_setting.flush_interval':
            settings['perf_metrics_setting.flush_interval'] ?? 5,
          'perf_metrics_setting.bucket_time':
            settings['perf_metrics_setting.bucket_time'] ?? 'hour',
          'perf_metrics_setting.retention_days':
            settings['perf_metrics_setting.retention_days'] ?? 0,
          'perf_metrics_setting.success_rate_green_threshold':
            settings['perf_metrics_setting.success_rate_green_threshold'] ??
            99.9,
          'perf_metrics_setting.success_rate_yellow_threshold':
            settings['perf_metrics_setting.success_rate_yellow_threshold'] ?? 99,
          'perf_metrics_setting.error_code_whitelist':
            settings['perf_metrics_setting.error_code_whitelist'] ?? '',
          'perf_metrics_setting.no_data_as_full':
            settings['perf_metrics_setting.no_data_as_full'] ?? true,
        }}
      />
    ),
  },
  {
    id: 'email',
    titleKey: 'SMTP Email',
    build: (settings: OperationsSettings) => (
      <EmailSettingsSection
        defaultValues={{
          SMTPServer: settings.SMTPServer,
          SMTPPort: settings.SMTPPort,
          SMTPAccount: settings.SMTPAccount,
          SMTPFrom: settings.SMTPFrom,
          SMTPToken: settings.SMTPToken,
          SMTPSSLEnabled: settings.SMTPSSLEnabled,
          SMTPStartTLSEnabled: settings.SMTPStartTLSEnabled,
          SMTPInsecureSkipVerify: settings.SMTPInsecureSkipVerify,
          SMTPForceAuthLogin: settings.SMTPForceAuthLogin,
        }}
      />
    ),
  },
  {
    id: 'email-promotion',
    titleKey: 'Email Promotion',
    build: (settings: OperationsSettings) => (
      <EmailPromotionSection
        defaultValues={{
          announcementEmailEnabled:
            settings['email_promotion_setting.announcement_email_enabled'],
          ratePerMinute: settings['email_promotion_setting.rate_per_minute'],
        }}
      />
    ),
  },
  {
    id: 'marketing-smtp',
    titleKey: 'Marketing SMTP',
    build: (settings: OperationsSettings) => (
      <MarketingSmtpSection
        defaultValues={{
          MarketingSMTPServer: settings.MarketingSMTPServer,
          MarketingSMTPPort: settings.MarketingSMTPPort,
          MarketingSMTPAccount: settings.MarketingSMTPAccount,
          MarketingSMTPFrom: settings.MarketingSMTPFrom,
          MarketingSMTPToken: settings.MarketingSMTPToken,
          MarketingSMTPSSLEnabled: settings.MarketingSMTPSSLEnabled,
          MarketingSMTPStartTLSEnabled: settings.MarketingSMTPStartTLSEnabled,
          MarketingSMTPInsecureSkipVerify:
            settings.MarketingSMTPInsecureSkipVerify,
          MarketingSMTPForceAuthLogin: settings.MarketingSMTPForceAuthLogin,
        }}
      />
    ),
  },
  {
    id: 'email-suppression',
    titleKey: 'Email Suppression',
    build: (settings: OperationsSettings) => (
      <EmailSuppressionSection
        defaultValues={{
          EmailDeliveryEventToken: settings.EmailDeliveryEventToken,
        }}
      />
    ),
  },
  {
    id: 'tickets',
    titleKey: 'Ticket System',
    build: (settings: OperationsSettings) => (
      <TicketSection
        defaultValues={{
          enabled: settings['ticket_setting.enabled'],
          adminNotifyEnabled: settings['ticket_setting.admin_notify_enabled'],
          attachmentMaxSizeMb:
            settings['ticket_setting.attachment_max_size_mb'],
          maxAttachmentsPerMessage:
            settings['ticket_setting.max_attachments_per_message'],
          attachmentRetentionDays:
            settings['ticket_setting.attachment_retention_days'],
          closedTicketRetentionDays:
            settings['ticket_setting.closed_ticket_retention_days'],
          allowedMimeTypes: settings['ticket_setting.allowed_mime_types'],
          types: settings['ticket_setting.types'],
        }}
      />
    ),
  },
  {
    id: 'worker',
    titleKey: 'Worker Proxy',
    build: (settings: OperationsSettings) => (
      <WorkerSettingsSection
        defaultValues={{
          WorkerUrl: settings.WorkerUrl,
          WorkerValidKey: settings.WorkerValidKey,
          WorkerAllowHttpImageRequestEnabled:
            settings.WorkerAllowHttpImageRequestEnabled,
        }}
      />
    ),
  },
  {
    id: 'logs',
    titleKey: 'Log Maintenance',
    build: (settings: OperationsSettings) => (
      <LogSettingsSection
        defaultEnabled={Boolean(settings.LogConsumeEnabled)}
      />
    ),
  },
  {
    id: 'performance',
    titleKey: 'Performance',
    build: (settings: OperationsSettings) => (
      <PerformanceSection
        defaultValues={{
          'performance_setting.disk_cache_enabled':
            settings['performance_setting.disk_cache_enabled'] ?? false,
          'performance_setting.disk_cache_threshold_mb':
            settings['performance_setting.disk_cache_threshold_mb'] ?? 10,
          'performance_setting.disk_cache_max_size_mb':
            settings['performance_setting.disk_cache_max_size_mb'] ?? 1024,
          'performance_setting.disk_cache_path':
            settings['performance_setting.disk_cache_path'] ?? '',
          'performance_setting.monitor_enabled':
            settings['performance_setting.monitor_enabled'] ?? false,
          'performance_setting.monitor_cpu_threshold':
            settings['performance_setting.monitor_cpu_threshold'] ?? 90,
          'performance_setting.monitor_memory_threshold':
            settings['performance_setting.monitor_memory_threshold'] ?? 90,
          'performance_setting.monitor_disk_threshold':
            settings['performance_setting.monitor_disk_threshold'] ?? 95,
        }}
      />
    ),
  },
  {
    id: 'update-checker',
    titleKey: 'System maintenance',
    build: (
      _settings: OperationsSettings,
      currentVersion?: string | null,
      startTime?: number | null
    ) => (
      <UpdateCheckerSection
        currentVersion={currentVersion}
        startTime={startTime}
      />
    ),
  },
] as const

export type OperationsSectionId = (typeof OPERATIONS_SECTIONS)[number]['id']

const operationsRegistry = createSectionRegistry<
  OperationsSectionId,
  OperationsSettings,
  [string | null | undefined, number | null | undefined]
>({
  sections: OPERATIONS_SECTIONS,
  defaultSection: 'behavior',
  basePath: '/system-settings/operations',
  urlStyle: 'path',
})

export const OPERATIONS_SECTION_IDS = operationsRegistry.sectionIds
export const OPERATIONS_DEFAULT_SECTION = operationsRegistry.defaultSection
export const getOperationsSectionNavItems =
  operationsRegistry.getSectionNavItems
export const getOperationsSectionContent = operationsRegistry.getSectionContent
export const getOperationsSectionMeta = operationsRegistry.getSectionMeta

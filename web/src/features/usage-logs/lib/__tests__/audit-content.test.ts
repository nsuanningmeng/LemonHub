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
import { describe, expect, test } from 'vitest'

import { createLocaleI18n } from '@/i18n/__tests__/helpers'

import type { LogOtherData } from '../../types'
import { renderAuditContent } from '../format'

type AuditCase = [
  string,
  NonNullable<LogOtherData['op']>['params'],
  string,
  string,
]

const cases: AuditCase[] = [
  [
    'user.request_body_record',
    {
      state: 'Enabled',
      username: 'Alice <admin>',
      id: '7',
    },
    '已启用用户 Alice <admin>（ID：7）的请求体记录',
    'Enabled request body recording for user Alice <admin> (ID: 7)',
  ],
  [
    'log.request_body_view',
    {
      request_id: 'req-model-x',
    },
    '查看请求 req-model-x 的已记录请求体',
    'Viewed the recorded request body for request req-model-x',
  ],
  [
    'log.request_body_clear',
    {
      deleted: 3,
    },
    '清除全部已记录请求体（移除 3 条）',
    'Cleared all recorded request bodies (3 removed)',
  ],
  [
    'site.create',
    {
      name: 'Site A',
      id: 2,
    },
    '创建子站 Site A（ID：2）',
    'Created sub-site Site A (ID: 2)',
  ],
  [
    'site.update',
    {
      name: 'Site A',
      id: 2,
    },
    '更新子站 Site A（ID：2）',
    'Updated sub-site Site A (ID: 2)',
  ],
  [
    'site.delete',
    {
      id: 2,
    },
    '删除子站（ID：2）',
    'Deleted sub-site (ID: 2)',
  ],
  [
    'site.wallet_recharge',
    {
      id: 2,
      amount: 1250,
    },
    '为子站 2 钱包充值 1250 厘',
    'Recharged sub-site 2 wallet by 1250 (1/1000 CNY)',
  ],
  [
    'site.wallet_adjust',
    {
      id: 2,
      amount: -50,
    },
    '调整子站 2 钱包余额 -50 厘',
    'Adjusted sub-site 2 wallet by -50 (1/1000 CNY)',
  ],
  [
    'redemption.void',
    {
      id: 9,
    },
    '作废兑换码（ID：9）并退还成本',
    'Voided redemption (ID: 9) and refunded its cost',
  ],
  [
    'site_admin.branding_update',
    {
      id: 2,
      name: 'Site A',
    },
    '子站管理员更新子站 2 的品牌设置（名称：Site A）',
    'Sub-site admin updated branding for site 2 (name Site A)',
  ],
  [
    'site_admin.redemption_create',
    {
      count: 2,
      name: 'Welcome',
      quota: '$5',
    },
    '子站管理员创建 2 个名为 Welcome 的兑换码（每个 $5）',
    'Sub-site admin created 2 redemption codes named Welcome ($5 each)',
  ],
  [
    'site_admin.redemption_void',
    {
      id: 9,
    },
    '子站管理员作废兑换码（ID：9）',
    'Sub-site admin voided redemption (ID: 9)',
  ],
  [
    'site_admin.warn_threshold',
    {
      id: 2,
      threshold: 300,
    },
    '子站管理员将子站 2 的钱包预警阈值设为 300 厘',
    'Sub-site admin set wallet warning threshold to 300 (1/1000 CNY) for site 2',
  ],
  [
    'site_admin.pay_config',
    {
      id: 2,
    },
    '子站管理员更新子站 2 的收款配置',
    'Sub-site admin updated payment collection settings for site 2',
  ],
  [
    'topup.retry_settlement',
    {
      trade_no: 'order-req-23',
      status: 'success',
    },
    '重试结算充值订单 order-req-23（结果：成功）',
    'Retried settlement of top-up order-req-23 (result Success)',
  ],
  [
    'subscription.plan_reset',
    {
      plan_id: 8,
    },
    '重置套餐 8 的有效订阅',
    'Reset active subscriptions for plan 8',
  ],
  [
    'subscription.user_plan_reset',
    {
      plan_id: 8,
      target_user_id: 7,
    },
    '重置用户 7 在套餐 8 下的有效订阅',
    'Reset active plan 8 subscriptions for user 7',
  ],
  [
    'channel.status_update',
    {
      id: 4,
      status: 2,
    },
    '将渠道 4 的状态更新为手动禁用',
    'Updated channel 4 status to Manual Disabled',
  ],
  [
    'channel.status_update_batch',
    {
      count: 2,
      total: 3,
      status: 1,
    },
    '将 3 个渠道中的 2 个更新为已启用',
    'Updated 2 of 3 channels to Enabled',
  ],
  [
    'channel.upstream_detect_all',
    {
      task_id: 'systask_23',
    },
    '启动所有渠道的上游模型检测（任务：systask_23）',
    'Started upstream model detection for all channels (task: systask_23)',
  ],
  [
    'email_campaign.create',
    {
      campaign_id: 12,
      target_group: 'gpt-4.1',
      target_status: 1,
      source: 'manual',
    },
    '创建邮件群发活动（ID：12，分组：gpt-4.1，用户：已启用，来源：手动）',
    'Created email campaign (ID: 12, group: gpt-4.1, users: Enabled, source: Manual)',
  ],
  [
    'email_suppression.add',
    {
      email: 'fail@example.test',
      reason: 'complaint',
    },
    '将 fail@example.test 加入邮件拦截名单（原因：垃圾邮件投诉）',
    'Added fail@example.test to the email suppression list (reason: Spam complaint)',
  ],
  [
    'email_suppression.delete',
    {
      suppression_id: 2,
    },
    '移除邮件拦截记录（ID：2）',
    'Removed email suppression entry (ID: 2)',
  ],
  [
    'email_suppression.import',
    {
      imported: 2,
      failed: 1,
    },
    '导入邮件拦截名单（成功：2，失败：1）',
    'Imported email suppression entries (imported: 2, failed: 1)',
  ],
  [
    'site_admin.model_pricing',
    {
      id: 2,
      model_price_rate: 12500,
    },
    '子站管理员将子站 2 的模型加价率设为 12500/10000',
    'Sub-site admin set model pricing rate to 12500/10000 for site 2',
  ],
  [
    'ticket.update',
    {
      ticket_id: 6,
      status: 'awaiting_user',
      priority: 'high',
    },
    '更新工单 6（状态：等待回复，优先级：高）',
    'Updated ticket 6 (status: Awaiting Reply, priority: High)',
  ],
  [
    'ticket.cleanup',
    {
      deleted_tickets: 1,
      deleted_messages: 2,
      deleted_files: 3,
      deleted_rows: 4,
      failed_files: 0,
    },
    '清理工单数据（工单：1，消息：2，文件：3，附件记录：4，文件失败：0）',
    'Cleaned ticket data (tickets: 1, messages: 2, files: 3, attachment records: 4, failed files: 0)',
  ],
  [
    'user.aff_cash_payout',
    {
      inviter_id: 7,
      amount: 250000,
      payout_id: 12,
      note: 'customer text',
    },
    '记录邀请人 7 的现金结算（额度：250000，结算记录：12）',
    'Recorded affiliate cash payout for inviter 7 (quota: 250000, payout: 12)',
  ],
]

const controlledEnumCases: AuditCase[] = [
  [
    'user.manage',
    {
      action: 'disable',
      username: 'Alice',
      id: 7,
    },
    '对用户 Alice（ID: 7）执行 禁用',
    'Performed Disable on user Alice (ID: 7)',
  ],
  [
    'user.manage',
    {
      action: 'enable',
      username: 'Alice',
      id: 7,
    },
    '对用户 Alice（ID: 7）执行 启用',
    'Performed Enable on user Alice (ID: 7)',
  ],
  [
    'user.manage',
    {
      action: 'delete',
      username: 'Alice',
      id: 7,
    },
    '对用户 Alice（ID: 7）执行 删除',
    'Performed Delete on user Alice (ID: 7)',
  ],
  [
    'user.manage',
    {
      action: 'promote',
      username: 'Alice',
      id: 7,
    },
    '对用户 Alice（ID: 7）执行 提升',
    'Performed Promote on user Alice (ID: 7)',
  ],
  [
    'user.manage',
    {
      action: 'demote',
      username: 'Alice',
      id: 7,
    },
    '对用户 Alice（ID: 7）执行 降级',
    'Performed Demote on user Alice (ID: 7)',
  ],
  [
    'channel.multi_key_manage',
    {
      action: 'disable_key',
      id: 7,
    },
    '对渠道（ID: 7）执行多密钥管理操作 禁用',
    'Multi-key management Disable on channel (ID: 7)',
  ],
  [
    'channel.multi_key_manage',
    {
      action: 'enable_key',
      id: 7,
    },
    '对渠道（ID: 7）执行多密钥管理操作 启用',
    'Multi-key management Enable on channel (ID: 7)',
  ],
  [
    'channel.multi_key_manage',
    {
      action: 'enable_all_keys',
      id: 7,
    },
    '对渠道（ID: 7）执行多密钥管理操作 启用全部',
    'Multi-key management Enable All on channel (ID: 7)',
  ],
  [
    'channel.multi_key_manage',
    {
      action: 'disable_all_keys',
      id: 7,
    },
    '对渠道（ID: 7）执行多密钥管理操作 禁用全部',
    'Multi-key management Disable All on channel (ID: 7)',
  ],
  [
    'channel.multi_key_manage',
    {
      action: 'delete_key',
      id: 7,
    },
    '对渠道（ID: 7）执行多密钥管理操作 删除',
    'Multi-key management Delete on channel (ID: 7)',
  ],
  [
    'channel.multi_key_manage',
    {
      action: 'delete_disabled_keys',
      id: 7,
    },
    '对渠道（ID: 7）执行多密钥管理操作 删除自动禁用',
    'Multi-key management Delete Auto-Disabled on channel (ID: 7)',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'email',
      username: 'Alice',
    },
    '清除用户 Alice 的 邮箱 绑定',
    'Cleared Email binding for user Alice',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'github',
      username: 'Alice',
    },
    '清除用户 Alice 的 GitHub 绑定',
    'Cleared GitHub binding for user Alice',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'discord',
      username: 'Alice',
    },
    '清除用户 Alice 的 Discord 绑定',
    'Cleared Discord binding for user Alice',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'oidc',
      username: 'Alice',
    },
    '清除用户 Alice 的 OIDC 绑定',
    'Cleared OIDC binding for user Alice',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'wechat',
      username: 'Alice',
    },
    '清除用户 Alice 的 微信 绑定',
    'Cleared WeChat binding for user Alice',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'telegram',
      username: 'Alice',
    },
    '清除用户 Alice 的 Telegram 绑定',
    'Cleared Telegram binding for user Alice',
  ],
  [
    'user.binding_clear',
    {
      bindingType: 'linuxdo',
      username: 'Alice',
    },
    '清除用户 Alice 的 LinuxDO 绑定',
    'Cleared LinuxDO binding for user Alice',
  ],
  [
    'login',
    {
      method: 'password',
    },
    '登录成功（通过 密码）',
    'Logged in successfully via Password',
  ],
  [
    'login',
    {
      method: '2fa',
    },
    '登录成功（通过 双重身份验证）',
    'Logged in successfully via Two-factor Authentication',
  ],
  [
    'login',
    {
      method: 'passkey',
    },
    '登录成功（通过 Passkey）',
    'Logged in successfully via Passkey',
  ],
  [
    'login',
    {
      method: 'wechat',
    },
    '登录成功（通过 微信）',
    'Logged in successfully via WeChat',
  ],
  [
    'login',
    {
      method: 'telegram',
    },
    '登录成功（通过 Telegram）',
    'Logged in successfully via Telegram',
  ],
  [
    'login',
    {
      method: 'oauth',
    },
    '登录成功（通过 OAuth）',
    'Logged in successfully via OAuth',
  ],
  [
    'login',
    {
      method: 'unknown',
    },
    '登录成功（通过 未知）',
    'Logged in successfully via Unknown',
  ],
  [
    'login',
    {
      method: 'oauth:github',
    },
    '登录成功（通过 OAuth · GitHub）',
    'Logged in successfully via OAuth · GitHub',
  ],
  [
    'login',
    {
      method: 'oauth:discord',
    },
    '登录成功（通过 OAuth · Discord）',
    'Logged in successfully via OAuth · Discord',
  ],
  [
    'login',
    {
      method: 'oauth:oidc',
    },
    '登录成功（通过 OAuth · OIDC）',
    'Logged in successfully via OAuth · OIDC',
  ],
  [
    'login',
    {
      method: 'oauth:linuxdo',
    },
    '登录成功（通过 OAuth · LinuxDO）',
    'Logged in successfully via OAuth · LinuxDO',
  ],
]

describe('structured admin audit translations', () => {
  test.each([...cases, ...controlledEnumCases])(
    'localizes %s and updates its text after switching language',
    async (action, params, chinese, english) => {
      const instance = await createLocaleI18n('zhCN')
      const other: LogOtherData = { op: { action, params } }
      const translate = (
        key: string,
        options?: Record<string, unknown>
      ): string => {
        expect(instance.exists(key, { lng: 'zhCN', fallbackLng: false })).toBe(
          true
        )
        expect(instance.exists(key, { lng: 'en', fallbackLng: false })).toBe(
          true
        )
        return instance.t(key, options)
      }
      expect(renderAuditContent(other, translate)).toBe(chinese)
      await instance.changeLanguage('en')
      expect(renderAuditContent(other, translate)).toBe(english)
    }
  )

  test('preserves unknown enum values and exact custom login identifiers in both languages', async () => {
    const instance = await createLocaleI18n('zhCN')
    const unknownCases: AuditCase[] = [
      [
        'user.manage',
        { action: 'ACTION_CUSTOM', username: 'password{{MODEL_X}}', id: 7 },
        '对用户 password{{MODEL_X}}（ID: 7）执行 ACTION_CUSTOM',
        'Performed ACTION_CUSTOM on user password{{MODEL_X}} (ID: 7)',
      ],
      [
        'channel.multi_key_manage',
        { action: 'get_key_status', id: 7 },
        '对渠道（ID: 7）执行多密钥管理操作 get_key_status',
        'Multi-key management get_key_status on channel (ID: 7)',
      ],
      [
        'user.binding_clear',
        { bindingType: 'CustomSSO', username: 'Alice' },
        '清除用户 Alice 的 CustomSSO 绑定',
        'Cleared CustomSSO binding for user Alice',
      ],
      ...[
        'oauth:AcmeTenant',
        'oauth:GitHub',
        ' password ',
        'PASSWORD',
        '__proto__',
        'constructor',
      ].map(
        (method): AuditCase => [
          'login',
          { method },
          `登录成功（通过 ${method}）`,
          `Logged in successfully via ${method}`,
        ]
      ),
    ]
    for (const [language, resultIndex] of [
      ['zhCN', 2],
      ['en', 3],
    ] as const) {
      await instance.changeLanguage(language)
      for (const row of unknownCases) {
        const [action, params] = row
        const other: LogOtherData = { op: { action, params } }
        const original = JSON.stringify(other)
        expect(renderAuditContent(other, instance.t)).toBe(row[resultIndex])
        expect(JSON.stringify(other)).toBe(original)
      }
    }
  })

  test('keeps unknown actions available for raw-content fallback', async () => {
    const instance = await createLocaleI18n('zhCN')
    for (const action of ['future.raw_error', '__proto__', 'constructor']) {
      expect(
        renderAuditContent(
          { op: { action, params: { error: 'MODEL_X: timeout req_123' } } },
          instance.t
        )
      ).toBeNull()
    }
    expect(renderAuditContent({}, instance.t)).toBeNull()
    expect(renderAuditContent(null, instance.t)).toBeNull()
  })

  test('translates only the controlled status and preserves raw identifiers and metadata', async () => {
    const instance = await createLocaleI18n('zhCN')
    const other: LogOtherData = {
      op: {
        action: 'topup.retry_settlement',
        params: {
          trade_no: 'success{{model-name}}<req_123>',
          status: 'success',
        },
      },
    }
    expect(renderAuditContent(other, instance.t)).toBe(
      '重试结算充值订单 success{{model-name}}<req_123>（结果：成功）'
    )
    expect(other.op?.params?.status).toBe('success')
    expect(
      renderAuditContent(
        {
          op: {
            action: 'topup.retry_settlement',
            params: { trade_no: 'order-1', status: 'UPSTREAM_UNKNOWN' },
          },
        },
        instance.t
      )
    ).toBe('重试结算充值订单 order-1（结果：UPSTREAM_UNKNOWN）')
  })

  test('uses the English singular form for one redemption without changing the Chinese count', async () => {
    const instance = await createLocaleI18n('en')
    const other: LogOtherData = {
      op: {
        action: 'site_admin.redemption_create',
        params: { count: 1, name: 'Welcome', quota: '$5' },
      },
    }
    expect(renderAuditContent(other, instance.t)).toBe(
      'Sub-site admin created 1 redemption code named Welcome ($5 each)'
    )
    await instance.changeLanguage('zhCN')
    expect(renderAuditContent(other, instance.t)).toBe(
      '子站管理员创建 1 个名为 Welcome 的兑换码（每个 $5）'
    )
  })
})

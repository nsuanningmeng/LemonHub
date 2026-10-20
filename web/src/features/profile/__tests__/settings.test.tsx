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
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'

import { NotificationTab } from '../components/tabs/notification-tab'
import type { UserProfile } from '../types'

const profile: UserProfile = {
  id: 1,
  username: 'alice',
  display_name: 'Alice',
  role: 1,
  group: 'default',
  quota: 1000000,
  used_quota: 0,
  request_count: 0,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
}
afterEach(() => vi.restoreAllMocks())

/** The body a PUT sends over the wire: undefined fields are left out. */
function sentBody(put: { mock: { calls: unknown[][] } }): unknown {
  const serializedBody = JSON.stringify(put.mock.calls[0][1])
  return JSON.parse(serializedBody)
}

describe('notifications a user receives', () => {
  function renderNotificationTab(
    role: number,
    setting: Record<string, unknown>,
    quotaRemindEnabled = true
  ) {
    const onUpdate = vi.fn()
    render(
      <NotificationTab
        profile={{ ...profile, role, setting: JSON.stringify(setting) }}
        onUpdate={onUpdate}
        quotaRemindEnabled={quotaRemindEnabled}
        quotaRemindThreshold={1000}
      />
    )
    return onUpdate
  }

  function mockSave(_role: number, _setting: Record<string, unknown>) {
    return vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  }

  async function save(onUpdate: () => void) {
    fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() => expect(onUpdate).toHaveBeenCalled())
  }

  it.each([
    { name: 'user', role: ROLE.USER, channel: undefined },
    { name: 'site admin', role: ROLE.SITE_ADMIN, channel: undefined },
    { name: 'admin', role: ROLE.ADMIN, channel: false },
    { name: 'root', role: ROLE.SUPER_ADMIN, channel: true },
  ])(
    'a $name who never chose sees the reminder on and the channel switch as $channel',
    (row) => {
      renderNotificationTab(row.role, { notify_type: 'email' })
      expect(
        screen.getByRole('switch', { name: 'Low balance reminder' })
      ).toBeChecked()
      const channel = screen.queryByRole('switch', {
        name: 'Channel notifications',
      })
      const upstream = screen.queryByRole('switch', {
        name: 'Upstream Model Updates',
      })
      if (row.channel === undefined) {
        expect(channel).not.toBeInTheDocument()
        expect(upstream).not.toBeInTheDocument()
        return
      }
      expect(channel?.getAttribute('aria-checked')).toBe(String(row.channel))
      expect(upstream).not.toBeChecked()
    }
  )

  it('a user on a site with reminders off sees one line instead of notification settings', () => {
    renderNotificationTab(ROLE.USER, { notify_type: 'email' }, false)
    expect(
      screen.getByText('No notifications are available to you right now')
    ).toBeInTheDocument()
    expect(screen.queryByText('Notification Method')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('switch', { name: 'Low balance reminder' })
    ).not.toBeInTheDocument()
  })

  it('an admin on a site with reminders off keeps only the administrator notifications', () => {
    renderNotificationTab(ROLE.ADMIN, { notify_type: 'email' }, false)
    expect(
      screen.queryByRole('switch', { name: 'Low balance reminder' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Channel notifications' })
    ).toBeInTheDocument()
    expect(screen.getByText('Notification Method')).toBeInTheDocument()
  })

  it('an empty threshold shows the site default and saves as the site default', async () => {
    const put = mockSave(ROLE.USER, { notify_type: 'email' })
    const onUpdate = renderNotificationTab(ROLE.USER, { notify_type: 'email' })
    const threshold = screen.getByRole('spinbutton', {
      name: 'Reminder threshold (USD)',
    })
    expect(threshold).toHaveValue(null)
    expect(threshold).toHaveAttribute('placeholder', '0.002')
    expect(
      screen.getByText('Leave empty to use the site default ($0.002)')
    ).toBeInTheDocument()
    await save(onUpdate)
    expect(sentBody(put)).toMatchObject({ quota_warning_threshold: 0 })
  })

  it('an untouched threshold is saved as stored, in quota units', async () => {
    const setting = { notify_type: 'email', quota_warning_threshold: 1234 }
    const put = mockSave(ROLE.USER, setting)
    const onUpdate = renderNotificationTab(ROLE.USER, setting)
    expect(
      screen.getByRole('spinbutton', { name: 'Reminder threshold (USD)' })
    ).toHaveValue(0.002468)
    await save(onUpdate)
    expect(sentBody(put)).toMatchObject({ quota_warning_threshold: 1234 })
  })

  it('turning the reminder off hides its threshold and saves the switch', async () => {
    const put = mockSave(ROLE.USER, { notify_type: 'email' })
    const onUpdate = renderNotificationTab(ROLE.USER, { notify_type: 'email' })
    fireEvent.click(
      screen.getByRole('switch', { name: 'Low balance reminder' })
    )
    expect(
      screen.queryByRole('spinbutton', { name: 'Reminder threshold (USD)' })
    ).not.toBeInTheDocument()
    await save(onUpdate)
    expect(sentBody(put)).toMatchObject({ quota_warning_enabled: false })
  })

  it('switches root never touched stay unset when it saves', async () => {
    const put = mockSave(ROLE.SUPER_ADMIN, { notify_type: 'email' })
    const onUpdate = renderNotificationTab(ROLE.SUPER_ADMIN, {
      notify_type: 'email',
    })
    await save(onUpdate)
    expect(sentBody(put)).not.toHaveProperty('channel_notify_enabled')
    expect(sentBody(put)).not.toHaveProperty('quota_warning_enabled')
  })

  it('an admin turning channel notifications on saves the switch', async () => {
    const put = mockSave(ROLE.ADMIN, { notify_type: 'email' })
    const onUpdate = renderNotificationTab(ROLE.ADMIN, { notify_type: 'email' })
    fireEvent.click(
      screen.getByRole('switch', { name: 'Channel notifications' })
    )
    await save(onUpdate)
    expect(sentBody(put)).toMatchObject({ channel_notify_enabled: true })
  })

  it('saving notifications retains the local marketing and IP recording preferences', async () => {
    const setting = { marketing_email_disabled: true, record_ip_log: true }
    const put = mockSave(ROLE.USER, setting)
    const onUpdate = renderNotificationTab(ROLE.USER, setting)
    expect(
      screen.getByRole('switch', { name: 'Receive promotional emails' })
    ).not.toBeChecked()
    expect(
      screen.getByRole('switch', { name: 'Record IP Address' })
    ).toBeChecked()
    await save(onUpdate)
    expect(sentBody(put)).toMatchObject(setting)
  })

  it('when reminders are disabled local marketing and IP preferences remain editable', async () => {
    const put = mockSave(ROLE.SITE_ADMIN, {})
    const onUpdate = renderNotificationTab(ROLE.SITE_ADMIN, {}, false)
    fireEvent.click(
      screen.getByRole('switch', { name: 'Receive promotional emails' })
    )
    fireEvent.click(screen.getByRole('switch', { name: 'Record IP Address' }))
    await save(onUpdate)
    expect(sentBody(put)).toMatchObject({
      marketing_email_disabled: true,
      record_ip_log: true,
    })
    expect(sentBody(put)).not.toHaveProperty('channel_notify_enabled')
  })

  it('a rejected notification save keeps the form available without reporting an update', async () => {
    const showError = vi.spyOn(toast, 'error')
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: false, message: 'Unavailable' } })
    const onUpdate = renderNotificationTab(ROLE.USER, {})
    fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() => expect(put).toHaveBeenCalled())
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Save Settings' })
      ).toBeEnabled()
    )
    expect(showError).toHaveBeenCalledWith('Unavailable')
    expect(onUpdate).not.toHaveBeenCalled()
  })
})

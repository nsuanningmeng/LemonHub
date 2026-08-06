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
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const createMarketingSmtpSchema = (t: (key: string) => string) =>
  z.object({
    MarketingSMTPServer: z.string(),
    MarketingSMTPPort: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^\d+$/.test(trimmed)
    }, t('Port must be a positive integer')),
    MarketingSMTPAccount: z.string(),
    MarketingSMTPFrom: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)
    }, t('Enter a valid email or leave blank')),
    MarketingSMTPToken: z.string(),
    MarketingSMTPSSLEnabled: z.boolean(),
    MarketingSMTPStartTLSEnabled: z.boolean(),
    MarketingSMTPInsecureSkipVerify: z.boolean(),
    MarketingSMTPForceAuthLogin: z.boolean(),
  })

type MarketingSmtpFormValues = z.infer<
  ReturnType<typeof createMarketingSmtpSchema>
>

type MarketingSmtpSectionProps = {
  defaultValues: MarketingSmtpFormValues
}

type SmtpSecurityMode = 'none' | 'ssl_tls' | 'starttls'

function getSmtpSecurityMode(values: {
  MarketingSMTPSSLEnabled: boolean
  MarketingSMTPStartTLSEnabled: boolean
}): SmtpSecurityMode {
  if (values.MarketingSMTPSSLEnabled) return 'ssl_tls'
  if (values.MarketingSMTPStartTLSEnabled) return 'starttls'
  return 'none'
}

export function MarketingSmtpSection({
  defaultValues,
}: MarketingSmtpSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const marketingSmtpSchema = createMarketingSmtpSchema(t)

  const form = useForm<MarketingSmtpFormValues>({
    resolver: zodResolver(marketingSmtpSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: MarketingSmtpFormValues) => {
    const securityMode = getSmtpSecurityMode(values)
    const sanitized = {
      MarketingSMTPServer: values.MarketingSMTPServer.trim(),
      MarketingSMTPPort: values.MarketingSMTPPort.trim(),
      MarketingSMTPAccount: values.MarketingSMTPAccount.trim(),
      MarketingSMTPFrom: values.MarketingSMTPFrom.trim(),
      MarketingSMTPToken: values.MarketingSMTPToken.trim(),
      MarketingSMTPSSLEnabled: securityMode === 'ssl_tls',
      MarketingSMTPStartTLSEnabled: securityMode === 'starttls',
      MarketingSMTPInsecureSkipVerify: values.MarketingSMTPInsecureSkipVerify,
      MarketingSMTPForceAuthLogin: values.MarketingSMTPForceAuthLogin,
    }

    const initial = {
      MarketingSMTPServer: defaultValues.MarketingSMTPServer.trim(),
      MarketingSMTPPort: defaultValues.MarketingSMTPPort.trim(),
      MarketingSMTPAccount: defaultValues.MarketingSMTPAccount.trim(),
      MarketingSMTPFrom: defaultValues.MarketingSMTPFrom.trim(),
      MarketingSMTPToken: defaultValues.MarketingSMTPToken.trim(),
      MarketingSMTPSSLEnabled: defaultValues.MarketingSMTPSSLEnabled,
      MarketingSMTPStartTLSEnabled: defaultValues.MarketingSMTPStartTLSEnabled,
      MarketingSMTPInsecureSkipVerify:
        defaultValues.MarketingSMTPInsecureSkipVerify,
      MarketingSMTPForceAuthLogin: defaultValues.MarketingSMTPForceAuthLogin,
    }

    const updates: Array<{ key: string; value: string | boolean }> = []

    if (sanitized.MarketingSMTPServer !== initial.MarketingSMTPServer) {
      updates.push({
        key: 'MarketingSMTPServer',
        value: sanitized.MarketingSMTPServer,
      })
    }

    if (sanitized.MarketingSMTPPort !== initial.MarketingSMTPPort) {
      updates.push({
        key: 'MarketingSMTPPort',
        value: sanitized.MarketingSMTPPort,
      })
    }

    if (sanitized.MarketingSMTPAccount !== initial.MarketingSMTPAccount) {
      updates.push({
        key: 'MarketingSMTPAccount',
        value: sanitized.MarketingSMTPAccount,
      })
    }

    if (sanitized.MarketingSMTPFrom !== initial.MarketingSMTPFrom) {
      updates.push({
        key: 'MarketingSMTPFrom',
        value: sanitized.MarketingSMTPFrom,
      })
    }

    if (
      sanitized.MarketingSMTPToken &&
      sanitized.MarketingSMTPToken !== initial.MarketingSMTPToken
    ) {
      updates.push({
        key: 'MarketingSMTPToken',
        value: sanitized.MarketingSMTPToken,
      })
    }

    if (sanitized.MarketingSMTPSSLEnabled !== initial.MarketingSMTPSSLEnabled) {
      updates.push({
        key: 'MarketingSMTPSSLEnabled',
        value: sanitized.MarketingSMTPSSLEnabled,
      })
    }

    if (
      sanitized.MarketingSMTPStartTLSEnabled !==
      initial.MarketingSMTPStartTLSEnabled
    ) {
      updates.push({
        key: 'MarketingSMTPStartTLSEnabled',
        value: sanitized.MarketingSMTPStartTLSEnabled,
      })
    }

    if (
      sanitized.MarketingSMTPInsecureSkipVerify !==
      initial.MarketingSMTPInsecureSkipVerify
    ) {
      updates.push({
        key: 'MarketingSMTPInsecureSkipVerify',
        value: sanitized.MarketingSMTPInsecureSkipVerify,
      })
    }

    if (
      sanitized.MarketingSMTPForceAuthLogin !==
      initial.MarketingSMTPForceAuthLogin
    ) {
      updates.push({
        key: 'MarketingSMTPForceAuthLogin',
        value: sanitized.MarketingSMTPForceAuthLogin,
      })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Marketing SMTP')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Dedicated SMTP for bulk and marketing email, kept separate from verification-code email. Leave the host empty to send bulk email through the main SMTP settings.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save marketing SMTP settings'
          />
          <FormField
            control={form.control}
            name='MarketingSMTPServer'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('SMTP Host')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder={t('smtp.example.com')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave empty to use the main SMTP settings for bulk email')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='MarketingSMTPPort'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Port')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete='off'
                      type='number'
                      placeholder='587'
                      {...field}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Common ports include 25, 465, and 587')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormItem>
              <FormLabel>{t('SMTP encryption')}</FormLabel>
              <FormControl>
                <RadioGroup
                  value={getSmtpSecurityMode({
                    MarketingSMTPSSLEnabled: form.watch(
                      'MarketingSMTPSSLEnabled'
                    ),
                    MarketingSMTPStartTLSEnabled: form.watch(
                      'MarketingSMTPStartTLSEnabled'
                    ),
                  })}
                  onValueChange={(value) => {
                    const mode = value as SmtpSecurityMode
                    form.setValue('MarketingSMTPSSLEnabled', mode === 'ssl_tls', {
                      shouldDirty: true,
                    })
                    form.setValue(
                      'MarketingSMTPStartTLSEnabled',
                      mode === 'starttls',
                      {
                        shouldDirty: true,
                      }
                    )
                  }}
                  className='gap-3'
                >
                  <div className='flex items-center gap-2'>
                    <RadioGroupItem
                      value='none'
                      id='marketing-smtp-security-none'
                    />
                    <Label
                      htmlFor='marketing-smtp-security-none'
                      className='cursor-pointer font-normal'
                    >
                      {t('No encryption')}
                    </Label>
                  </div>
                  <div className='flex items-center gap-2'>
                    <RadioGroupItem
                      value='ssl_tls'
                      id='marketing-smtp-security-ssl-tls'
                    />
                    <Label
                      htmlFor='marketing-smtp-security-ssl-tls'
                      className='cursor-pointer font-normal'
                    >
                      {t('SSL/TLS')}
                    </Label>
                  </div>
                  <div className='flex items-center gap-2'>
                    <RadioGroupItem
                      value='starttls'
                      id='marketing-smtp-security-starttls'
                    />
                    <Label
                      htmlFor='marketing-smtp-security-starttls'
                      className='cursor-pointer font-normal'
                    >
                      {t('STARTTLS')}
                    </Label>
                  </div>
                </RadioGroup>
              </FormControl>
              <FormDescription>
                {t('Choose one SMTP transport security mode')}
              </FormDescription>
            </FormItem>

            <FormField
              control={form.control}
              name='MarketingSMTPInsecureSkipVerify'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Skip SMTP TLS certificate verification')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Allow self-signed or hostname-mismatched SMTP certificates'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='MarketingSMTPForceAuthLogin'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Force AUTH LOGIN')}</FormLabel>
                    <FormDescription>
                      {t('Force SMTP authentication using AUTH LOGIN method')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='MarketingSMTPAccount'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Username')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder={t('noreply@example.com')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Account used when authenticating with the SMTP server')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='MarketingSMTPFrom'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('From Address')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder={t('New API &lt;noreply@example.com&gt;')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Display name and email used in outgoing messages')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='MarketingSMTPToken'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Password / Access Token')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    type='password'
                    placeholder={t('Enter new token to update')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave blank to keep the existing credential')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

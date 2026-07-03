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
import { useEffect } from 'react'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const botProtectionSchema = z.object({
  TurnstileCheckEnabled: z.boolean(),
  CaptchaProvider: z.enum(['turnstile', 'geetest', 'altcha', 'tencent']),
  TurnstileSiteKey: z.string().optional(),
  TurnstileSecretKey: z.string().optional(),
  GeetestCaptchaId: z.string().optional(),
  GeetestCaptchaKey: z.string().optional(),
  TencentCaptchaAppId: z.string().optional(),
  TencentCaptchaAppSecretKey: z.string().optional(),
  TencentCloudSecretId: z.string().optional(),
  TencentCloudSecretKey: z.string().optional(),
})

type BotProtectionFormValues = z.infer<typeof botProtectionSchema>

type BotProtectionSectionProps = {
  defaultValues: BotProtectionFormValues
}

function SecretInput({
  label,
  placeholder,
  ...field
}: {
  label: string
  placeholder: string
  value?: string
  onChange?: (...args: unknown[]) => void
}) {
  return (
    <FormItem>
      <FormLabel>{label}</FormLabel>
      <FormControl>
        <Input
          type='password'
          placeholder={placeholder}
          autoComplete='new-password'
          {...field}
        />
      </FormControl>
      <FormMessage />
    </FormItem>
  )
}

export function BotProtectionSection({
  defaultValues,
}: BotProtectionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<BotProtectionFormValues>({
    resolver: zodResolver(botProtectionSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const provider = form.watch('CaptchaProvider')

  const onSubmit = async (data: BotProtectionFormValues) => {
    const updates = Object.entries(data).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof BotProtectionFormValues]
    )

    // Apply the enable toggle LAST: the backend enable-guard checks whether the
    // *currently persisted* provider is configured, so the new provider and its
    // credentials must be saved before TurnstileCheckEnabled flips to true —
    // otherwise switching provider and enabling in one save is rejected.
    updates.sort(([a], [b]) => {
      if (a === 'TurnstileCheckEnabled') return 1
      if (b === 'TurnstileCheckEnabled') return -1
      return 0
    })

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Bot Protection')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='TurnstileCheckEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable human verification')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Protect login, registration and password reset with the selected verification channel'
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
            name='CaptchaProvider'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Verification channel')}</FormLabel>
                <FormControl>
                  <Select
                    items={[
                      { value: 'turnstile', label: 'Cloudflare Turnstile' },
                      { value: 'geetest', label: t('GeeTest v4') },
                      { value: 'altcha', label: t('ALTCHA (self-hosted)') },
                      {
                        value: 'tencent',
                        label: t('Tencent Cloud Captcha (International)'),
                      },
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={t('Select channel')} />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='turnstile'>
                          Cloudflare Turnstile
                        </SelectItem>
                        <SelectItem value='geetest'>
                          {t('GeeTest v4')}
                        </SelectItem>
                        <SelectItem value='altcha'>
                          {t('ALTCHA (self-hosted)')}
                        </SelectItem>
                        <SelectItem value='tencent'>
                          {t('Tencent Cloud Captcha (International)')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <FormDescription>
                  {t(
                    'GeeTest and Tencent load reliably in mainland China; ALTCHA is served from this deployment itself and needs no external account'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {provider === 'turnstile' && (
            <>
              <FormField
                control={form.control}
                name='TurnstileSiteKey'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Site Key')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Your Turnstile site key')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TurnstileSecretKey'
                render={({ field }) => (
                  <SecretInput
                    label={t('Secret Key')}
                    placeholder={t('Your Turnstile secret key')}
                    {...field}
                  />
                )}
              />
            </>
          )}

          {provider === 'geetest' && (
            <>
              <FormField
                control={form.control}
                name='GeetestCaptchaId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Captcha ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('GeeTest captcha_id from the GeeTest console')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='GeetestCaptchaKey'
                render={({ field }) => (
                  <SecretInput
                    label={t('Captcha Key')}
                    placeholder={t('GeeTest captcha_key from the GeeTest console')}
                    {...field}
                  />
                )}
              />
            </>
          )}

          {provider === 'altcha' && (
            <FormDescription>
              {t(
                'No configuration needed. Challenges are generated and verified by this server; the signing key is created automatically.'
              )}
            </FormDescription>
          )}

          {provider === 'tencent' && (
            <>
              <FormField
                control={form.control}
                name='TencentCaptchaAppId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Captcha App ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('CaptchaAppId from the Tencent Cloud captcha console')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentCaptchaAppSecretKey'
                render={({ field }) => (
                  <SecretInput
                    label={t('Captcha App Secret Key')}
                    placeholder={t('AppSecretKey from the Tencent Cloud captcha console')}
                    {...field}
                  />
                )}
              />
              <FormField
                control={form.control}
                name='TencentCloudSecretId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Cloud API Secret ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Tencent Cloud CAM SecretId used to sign verify calls')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='TencentCloudSecretKey'
                render={({ field }) => (
                  <SecretInput
                    label={t('Cloud API Secret Key')}
                    placeholder={t('Tencent Cloud CAM SecretKey used to sign verify calls')}
                    {...field}
                  />
                )}
              />
            </>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

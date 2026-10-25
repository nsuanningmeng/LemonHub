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
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

import type { CampaignFormValues } from './lib/campaign-form'

type CampaignFieldsProps = { form: UseFormReturn<CampaignFormValues> }

export function CampaignFields(props: CampaignFieldsProps) {
  const { t } = useTranslation()
  const rewardType = props.form.watch('type')
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return (
    <div className='grid gap-5 sm:grid-cols-2'>
      <FormField
        control={props.form.control}
        name='title'
        render={({ field }) => (
          <FormItem className='sm:col-span-2'>
            <FormLabel>{t('Campaign title')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                maxLength={120}
                placeholder={t('A little thank-you for your support')}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={props.form.control}
        name='description'
        render={({ field }) => (
          <FormItem className='sm:col-span-2'>
            <FormLabel>{t('Description')}</FormLabel>
            <FormControl>
              <Textarea
                {...field}
                maxLength={1000}
                rows={2}
                placeholder={t('A message to include with the reward')}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={props.form.control}
        name='type'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Reward type')}</FormLabel>
            <FormControl>
              <NativeSelect {...field} className='w-full'>
                <option value='fixed'>{t('Reward credit')}</option>
                <option value='random'>{t('Lucky red packet')}</option>
              </NativeSelect>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={props.form.control}
        name='audience'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Eligible users')}</FormLabel>
            <FormControl>
              <NativeSelect {...field} className='w-full'>
                <option value='topup'>
                  {t('Users with successful top-ups')}
                </option>
                <option value='usage'>
                  {t('Users with successful API calls')}
                </option>
                <option value='both'>
                  {t('Users with successful top-ups and API calls')}
                </option>
                <option value='either'>
                  {t('Users with successful top-ups or API calls')}
                </option>
              </NativeSelect>
            </FormControl>
            <FormMessage />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Recipients must be enabled. Within the selected period, “and” requires both a successful top-up and a successful API call; “or” requires either activity.'
              )}
            </p>
          </FormItem>
        )}
      />
      <FormField
        control={props.form.control}
        name='minimum'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {rewardType === 'fixed'
                ? t('Amount per user (USD)')
                : t('Minimum amount (USD)')}
            </FormLabel>
            <FormControl>
              <Input
                {...field}
                type='number'
                inputMode='decimal'
                min='0'
                step='any'
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      {rewardType === 'random' && (
        <FormField
          control={props.form.control}
          name='maximum'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Maximum amount (USD)')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type='number'
                  inputMode='decimal'
                  min='0'
                  step='any'
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
      <p className='text-muted-foreground text-xs sm:col-span-2'>
        {t(
          'Rewards are credited to the account balance when claimed. Lucky amounts are drawn within the configured range.'
        )}
      </p>
      <FormField
        control={props.form.control}
        name='expires_in_days'
        render={({ field }) => (
          <FormItem className='sm:col-span-2'>
            <FormLabel>{t('Reward validity (days)')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                value={Number.isNaN(field.value) ? '' : field.value}
                onChange={(event) => {
                  field.onChange(
                    event.target.value === ''
                      ? Number.NaN
                      : Number(event.target.value)
                  )
                }}
                type='number'
                inputMode='numeric'
                min={1}
                max={3650}
                step={1}
                className='sm:max-w-48'
              />
            </FormControl>
            <FormDescription>
              {t(
                'Validity starts when the campaign is issued and applies to both reward credits and lucky red packets. Unclaimed rewards expire after this period.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={props.form.control}
        name='start'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Start time')}</FormLabel>
            <FormControl>
              <Input {...field} type='datetime-local' />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={props.form.control}
        name='end'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('End time')}</FormLabel>
            <FormControl>
              <Input {...field} type='datetime-local' />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <p className='text-muted-foreground text-xs sm:col-span-2'>
        {t(
          'Time zone: {{zone}}. The start time is included; the end time is excluded.',
          { zone: timeZone }
        )}{' '}
        {t(
          'Each eligible main-site user receives one reward per campaign. Only successful, unrefunded top-ups and successful API usage are included.'
        )}
      </p>
    </div>
  )
}

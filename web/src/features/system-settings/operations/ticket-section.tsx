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
import { useMemo } from 'react'
import * as z from 'zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import type { TicketTypeConfig } from '../types'

const COMMON_MIME_TYPES = [
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
] as const

const ticketTypeSchema = z.object({
  key: z.string(),
  name: z.string(),
  promptTemplate: z.string(),
  enabled: z.boolean(),
})

const ticketSchema = z.object({
  enabled: z.boolean(),
  adminNotifyEnabled: z.boolean(),
  attachmentMaxSizeMb: z.coerce.number().int().min(0),
  maxAttachmentsPerMessage: z.coerce.number().int().min(0),
  attachmentRetentionDays: z.coerce.number().int().min(0),
  closedTicketRetentionDays: z.coerce.number().int().min(0),
  allowedMimeTypes: z.array(z.string()),
  types: z.array(ticketTypeSchema).superRefine((types, ctx) => {
    const seen = new Set<string>()
    types.forEach((type, index) => {
      const key = type.key.trim()
      if (!key) {
        ctx.addIssue({
          code: 'custom',
          message: 'Type key is required',
          path: [index, 'key'],
        })
        return
      }
      if (seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Type key must be unique',
          path: [index, 'key'],
        })
        return
      }
      seen.add(key)
    })
  }),
})

type TicketFormInput = z.input<typeof ticketSchema>
type TicketFormValues = z.output<typeof ticketSchema>

type TicketSectionProps = {
  defaultValues: {
    enabled: boolean
    adminNotifyEnabled: boolean
    attachmentMaxSizeMb: number
    maxAttachmentsPerMessage: number
    attachmentRetentionDays: number
    closedTicketRetentionDays: number
    allowedMimeTypes: string[]
    types: TicketTypeConfig[]
  }
}

type PersistedTicketSettings = {
  'ticket_setting.enabled': boolean
  'ticket_setting.admin_notify_enabled': boolean
  'ticket_setting.attachment_max_size_mb': number
  'ticket_setting.max_attachments_per_message': number
  'ticket_setting.attachment_retention_days': number
  'ticket_setting.closed_ticket_retention_days': number
  'ticket_setting.allowed_mime_types': string
  'ticket_setting.types': string
}

function buildFormDefaults(
  defaults: TicketSectionProps['defaultValues']
): TicketFormInput {
  return {
    enabled: defaults.enabled,
    adminNotifyEnabled: defaults.adminNotifyEnabled,
    attachmentMaxSizeMb: defaults.attachmentMaxSizeMb,
    maxAttachmentsPerMessage: defaults.maxAttachmentsPerMessage,
    attachmentRetentionDays: defaults.attachmentRetentionDays,
    closedTicketRetentionDays: defaults.closedTicketRetentionDays,
    allowedMimeTypes: [...defaults.allowedMimeTypes],
    types: defaults.types.map((type) => ({
      key: type.key,
      name: type.name,
      promptTemplate: type.prompt_template,
      enabled: type.enabled,
    })),
  }
}

function toPersisted(values: {
  enabled: boolean
  adminNotifyEnabled: boolean
  attachmentMaxSizeMb: number
  maxAttachmentsPerMessage: number
  attachmentRetentionDays: number
  closedTicketRetentionDays: number
  allowedMimeTypes: string[]
  types: Array<{
    key: string
    name: string
    promptTemplate: string
    enabled: boolean
  }>
}): PersistedTicketSettings {
  const normalizedTypes = values.types.map((type) => ({
    key: type.key.trim(),
    name: type.name.trim(),
    prompt_template: type.promptTemplate,
    enabled: type.enabled,
  }))

  return {
    'ticket_setting.enabled': values.enabled,
    'ticket_setting.admin_notify_enabled': values.adminNotifyEnabled,
    'ticket_setting.attachment_max_size_mb': values.attachmentMaxSizeMb,
    'ticket_setting.max_attachments_per_message':
      values.maxAttachmentsPerMessage,
    'ticket_setting.attachment_retention_days': values.attachmentRetentionDays,
    'ticket_setting.closed_ticket_retention_days':
      values.closedTicketRetentionDays,
    'ticket_setting.allowed_mime_types': JSON.stringify(values.allowedMimeTypes),
    'ticket_setting.types': JSON.stringify(normalizedTypes),
  }
}

export function TicketSection({ defaultValues }: TicketSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<TicketFormInput, unknown, TicketFormValues>({
    resolver: zodResolver(ticketSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'types',
  })

  const ticketEnabled = form.watch('enabled')

  // Show the common image types plus any extra saved values so nothing is lost.
  const mimeOptions = useMemo(() => {
    const extras = defaultValues.allowedMimeTypes.filter(
      (value) => !COMMON_MIME_TYPES.includes(value as (typeof COMMON_MIME_TYPES)[number])
    )
    return [...COMMON_MIME_TYPES, ...extras]
  }, [defaultValues.allowedMimeTypes])

  const onSubmit = async (values: TicketFormValues) => {
    const baseline = toPersisted({
      ...defaultValues,
      types: defaultValues.types.map((type) => ({
        key: type.key,
        name: type.name,
        promptTemplate: type.prompt_template,
        enabled: type.enabled,
      })),
    })
    const next = toPersisted(values)

    const updates = (
      Object.keys(next) as Array<keyof PersistedTicketSettings>
    )
      .filter((key) => next[key] !== baseline[key])
      .map((key) => ({ key, value: next[key] }))

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Ticket System')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save ticket settings'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable ticket system')}</FormLabel>
                  <FormDescription>
                    {t('Let users open support tickets from the console.')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch checked={field.value} onCheckedChange={field.onChange} />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='adminNotifyEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Notify admins of new tickets')}</FormLabel>
                  <FormDescription>
                    {t('Send a notification to admins when a ticket is created.')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={!ticketEnabled}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='attachmentMaxSizeMb'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Attachment max size (MB)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ticketEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='maxAttachmentsPerMessage'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max attachments per message')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ticketEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='attachmentRetentionDays'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Attachment retention (days)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ticketEnabled}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('0 means attachments are never auto-deleted.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='closedTicketRetentionDays'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Closed ticket retention (days)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ticketEnabled}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      '0 means closed tickets and their messages are kept forever.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='allowedMimeTypes'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel>{t('Allowed attachment types')}</FormLabel>
                <div className='flex flex-wrap gap-4'>
                  {mimeOptions.map((mime) => {
                    const checked = field.value.includes(mime)
                    return (
                      <label
                        key={mime}
                        className='flex cursor-pointer items-center gap-2 text-sm'
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(value) => {
                            const isChecked = value === true
                            const nextValue = isChecked
                              ? [...field.value, mime]
                              : field.value.filter((item) => item !== mime)
                            field.onChange(nextValue)
                          }}
                          disabled={!ticketEnabled}
                        />
                        <span>{mime}</span>
                      </label>
                    )
                  })}
                </div>
                <FormDescription>
                  {t('MIME types users are allowed to attach to a ticket.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Card data-settings-form-span='full'>
            <CardHeader>
              <div className='flex items-center justify-between gap-2'>
                <CardTitle>{t('Ticket types')}</CardTitle>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    append({
                      key: '',
                      name: '',
                      promptTemplate: '',
                      enabled: true,
                    })
                  }
                  disabled={!ticketEnabled}
                >
                  <Plus data-icon='inline-start' />
                  <span>{t('Add type')}</span>
                </Button>
              </div>
            </CardHeader>
            <CardContent className='flex flex-col gap-4'>
              {fields.length === 0 ? (
                <p className='text-muted-foreground text-sm'>
                  {t('No ticket types yet. Add one to customize prompts.')}
                </p>
              ) : (
                fields.map((row, index) => (
                  <div
                    key={row.id}
                    className='flex flex-col gap-3 rounded-lg border p-3'
                  >
                    <div className='grid gap-3 md:grid-cols-2'>
                      <FormField
                        control={form.control}
                        name={`types.${index}.key`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Key')}</FormLabel>
                            <FormControl>
                              <Input
                                placeholder={t('e.g. billing')}
                                {...field}
                                onChange={(event) =>
                                  field.onChange(event.target.value)
                                }
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`types.${index}.name`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Name')}</FormLabel>
                            <FormControl>
                              <Input
                                placeholder={t('e.g. Billing issue')}
                                {...field}
                                onChange={(event) =>
                                  field.onChange(event.target.value)
                                }
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>

                    <FormField
                      control={form.control}
                      name={`types.${index}.promptTemplate`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Prompt template')}</FormLabel>
                          <FormControl>
                            <Textarea
                              className='min-h-20'
                              placeholder={t(
                                'Custom prompt template for this ticket type'
                              )}
                              {...field}
                              onChange={(event) =>
                                field.onChange(event.target.value)
                              }
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <div className='flex items-center justify-between gap-3'>
                      <FormField
                        control={form.control}
                        name={`types.${index}.enabled`}
                        render={({ field }) => (
                          <div className='flex items-center gap-2'>
                            <Switch
                              checked={field.value}
                              onCheckedChange={field.onChange}
                            />
                            <Label className='font-normal'>{t('Enabled')}</Label>
                          </div>
                        )}
                      />
                      <Button
                        type='button'
                        variant='destructive'
                        size='sm'
                        onClick={() => remove(index)}
                      >
                        <Trash2 data-icon='inline-start' />
                        <span>{t('Remove')}</span>
                      </Button>
                    </div>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

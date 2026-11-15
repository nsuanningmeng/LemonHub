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
import { useQuery } from '@tanstack/react-query'
import { Code2, Eye, RotateCcw, Save } from 'lucide-react'
import { memo, useCallback, useEffect, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { getEnabledModels } from '@/features/channels/api'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import {
  ModelRatioVisualEditor,
  type ModelRatioVisualEditorHandle,
} from './model-ratio-visual-editor'
import { PriceChangeDiff } from './price-change-diff'
import {
  PRICE_REVIEW_FIELDS,
  type ModelPriceChange,
  type UnreadablePriceChange,
} from './price-change-review'

type ModelFormValues = {
  ModelPrice: string
  ModelRatio: string
  CacheRatio: string
  CreateCacheRatio: string
  CompletionRatio: string
  ImageRatio: string
  AudioRatio: string
  AudioCompletionRatio: string
  ExposeRatioEnabled: boolean
  BillingMode: string
  BillingExpr: string
}

type ModelRatioFormProps = {
  form: UseFormReturn<ModelFormValues>
  savedValues: ModelFormValues
  onSave: (values: ModelFormValues) => Promise<void>
  onReset: () => void
  isSaving: boolean
  isResetting: boolean
  variant?: 'default' | 'unset'
}

type ModelJsonFieldName =
  | 'ModelPrice'
  | 'ModelRatio'
  | 'CacheRatio'
  | 'CreateCacheRatio'
  | 'CompletionRatio'
  | 'ImageRatio'
  | 'AudioRatio'
  | 'AudioCompletionRatio'

const modelJsonFields: Array<{
  name: ModelJsonFieldName
  labelKey: string
  descriptionKey: string
}> = [
  {
    name: 'ModelPrice',
    labelKey: 'Model fixed pricing',
    descriptionKey:
      'JSON map of model → USD cost per request. Takes precedence over ratio based billing.',
  },
  {
    name: 'ModelRatio',
    labelKey: 'Model ratio',
    descriptionKey: 'JSON map of model → multiplier applied to quota billing.',
  },
  {
    name: 'CacheRatio',
    labelKey: 'Prompt cache ratio',
    descriptionKey: 'Optional ratio used when upstream cache hits occur.',
  },
  {
    name: 'CreateCacheRatio',
    labelKey: 'Create cache ratio',
    descriptionKey:
      'Ratio applied when creating cache entries for supported models.',
  },
  {
    name: 'CompletionRatio',
    labelKey: 'Completion ratio',
    descriptionKey:
      'Applies to custom completion endpoints. JSON map of model → ratio.',
  },
  {
    name: 'ImageRatio',
    labelKey: 'Image ratio',
    descriptionKey: 'Configure per-model ratio for image inputs or outputs.',
  },
  {
    name: 'AudioRatio',
    labelKey: 'Audio ratio',
    descriptionKey:
      'Ratio applied to audio inputs where supported by the upstream model.',
  },
  {
    name: 'AudioCompletionRatio',
    labelKey: 'Audio completion ratio',
    descriptionKey: 'Ratio applied to audio completions for streaming models.',
  },
]

function ModelJsonTextareaField(props: {
  form: UseFormReturn<ModelFormValues>
  name: ModelJsonFieldName
  label: string
  description: string
}) {
  return (
    <FormField
      control={props.form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem className='flex min-w-0 flex-col gap-2'>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <JsonCodeEditor
              value={field.value}
              onChange={(value) => field.onChange(value)}
              name={field.name}
              onBlur={field.onBlur}
              textareaRef={field.ref}
            />
          </FormControl>
          <FormDescription className='text-xs leading-5'>
            {props.description}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export const ModelRatioForm = memo(function ModelRatioForm({
  form,
  savedValues,
  onSave,
  onReset,
  isSaving,
  isResetting,
  variant = 'default',
}: ModelRatioFormProps) {
  const { t } = useTranslation()
  const isUnsetVariant = variant === 'unset'
  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')
  const visualEditorRef = useRef<ModelRatioVisualEditorHandle>(null)
  const [pendingSave, setPendingSave] = useState<{
    values: ModelFormValues
    changes: ModelPriceChange[]
    unreadable: UnreadablePriceChange[]
    exposedBefore: boolean
  } | null>(null)
  const [confirming, setConfirming] = useState(false)
  const confirmPendingRef = useRef(false)

  const reviewSave = useCallback(
    (values: ModelFormValues) => {
      const byModel = new Map<string, ModelPriceChange>()
      const unreadable: UnreadablePriceChange[] = []
      for (const [field, label] of PRICE_REVIEW_FIELDS) {
        let draft: Record<string, unknown>
        try {
          draft = JSON.parse(values[field] || '{}')
          if (!draft || typeof draft !== 'object' || Array.isArray(draft)) {
            throw new Error()
          }
        } catch {
          toast.error(`${t(label)}: ${t('Invalid JSON')}`)
          return
        }
        let saved: Record<string, unknown>
        try {
          saved = JSON.parse(savedValues[field] || '{}')
          if (!saved || typeof saved !== 'object' || Array.isArray(saved)) {
            throw new Error()
          }
        } catch {
          unreadable.push({
            field,
            label,
            saved: savedValues[field],
            draft: values[field],
          })
          continue
        }
        const names = new Set([...Object.keys(saved), ...Object.keys(draft)])
        for (const name of names) {
          const before = Object.hasOwn(saved, name) ? saved[name] : undefined
          const after = Object.hasOwn(draft, name) ? draft[name] : undefined
          const beforeJSON = JSON.stringify(before)
          const afterJSON = JSON.stringify(after)
          if (beforeJSON === afterJSON) continue
          const change: ModelPriceChange = byModel.get(name) ?? {
            name,
            saved: {},
            draft: {},
          }
          // Expressions and billing modes are text; other fields show JSON
          // literals so a number and a malformed quoted number stay distinct.
          change.saved[field] =
            (field === 'BillingExpr' || field === 'BillingMode') &&
            typeof before === 'string' &&
            before !== ''
              ? before
              : beforeJSON
          change.draft[field] =
            (field === 'BillingExpr' || field === 'BillingMode') &&
            typeof after === 'string' &&
            after !== ''
              ? after
              : afterJSON
          byModel.set(name, change)
        }
      }
      const changes = [...byModel.values()].sort((a, b) =>
        a.name.localeCompare(b.name)
      )
      if (
        changes.length === 0 &&
        unreadable.length === 0 &&
        values.ExposeRatioEnabled === savedValues.ExposeRatioEnabled
      ) {
        toast.info(t('No model price changes to save'))
        return
      }
      setPendingSave({
        values: { ...values },
        changes,
        unreadable,
        exposedBefore: savedValues.ExposeRatioEnabled,
      })
    },
    [savedValues, t]
  )

  const confirmSave = async () => {
    if (!pendingSave || confirmPendingRef.current) return
    confirmPendingRef.current = true
    setConfirming(true)
    try {
      await onSave(pendingSave.values)
      setPendingSave(null)
    } catch {
      toast.error(t('Failed to save'))
    } finally {
      confirmPendingRef.current = false
      setConfirming(false)
    }
  }

  const enabledModelsQuery = useQuery({
    queryKey: ['enabled-models'],
    queryFn: getEnabledModels,
    enabled: isUnsetVariant,
  })

  const enabledModelsError = isUnsetVariant
    ? enabledModelsQuery.isError ||
      (enabledModelsQuery.data !== undefined &&
        !enabledModelsQuery.data.success)
    : false
  const enabledModelsErrorMessage = enabledModelsQuery.data?.message

  useEffect(() => {
    if (!enabledModelsError) return
    toast.error(enabledModelsErrorMessage || t('Failed to load enabled models'))
  }, [enabledModelsError, enabledModelsErrorMessage, t])

  const handleFieldChange = useCallback(
    (field: keyof ModelFormValues, value: string) => {
      form.setValue(field, value, {
        shouldValidate: true,
        shouldDirty: true,
      })
    },
    [form]
  )

  const toggleEditMode = useCallback(() => {
    setEditMode((prev) => (prev === 'visual' ? 'json' : 'visual'))
  }, [])

  const handleSave = useCallback(async () => {
    if (editMode === 'visual') {
      const committed = await visualEditorRef.current?.commitOpenEditor()
      if (committed === false) return
    }

    await form.handleSubmit(reviewSave)()
  }, [editMode, form, reviewSave])

  return (
    <div className='space-y-6'>
      {!isUnsetVariant && (
        <div className='flex flex-wrap justify-end gap-2'>
          <Button
            type='button'
            variant='destructive'
            size='sm'
            onClick={onReset}
            disabled={isResetting}
          >
            <RotateCcw data-icon='inline-start' />
            {t('Reset prices')}
          </Button>
          {editMode === 'json' && (
            <Button
              type='button'
              size='sm'
              onClick={handleSave}
              disabled={isSaving}
            >
              <Save data-icon='inline-start' />
              {isSaving ? t('Saving...') : t('Save model prices')}
            </Button>
          )}
          <Button variant='outline' size='sm' onClick={toggleEditMode}>
            {editMode === 'visual' ? (
              <>
                <Code2 className='mr-2 h-4 w-4' />
                {t('Switch to JSON')}
              </>
            ) : (
              <>
                <Eye className='mr-2 h-4 w-4' />
                {t('Switch to Visual')}
              </>
            )}
          </Button>
        </div>
      )}

      <Form {...form}>
        {editMode === 'visual' ? (
          <div className='space-y-6'>
            <ModelRatioVisualEditor
              ref={visualEditorRef}
              savedModelPrice={savedValues.ModelPrice}
              savedModelRatio={savedValues.ModelRatio}
              savedCacheRatio={savedValues.CacheRatio}
              savedCreateCacheRatio={savedValues.CreateCacheRatio}
              savedCompletionRatio={savedValues.CompletionRatio}
              savedImageRatio={savedValues.ImageRatio}
              savedAudioRatio={savedValues.AudioRatio}
              savedAudioCompletionRatio={savedValues.AudioCompletionRatio}
              savedBillingMode={savedValues.BillingMode}
              savedBillingExpr={savedValues.BillingExpr}
              modelPrice={form.watch('ModelPrice')}
              modelRatio={form.watch('ModelRatio')}
              cacheRatio={form.watch('CacheRatio')}
              createCacheRatio={form.watch('CreateCacheRatio')}
              completionRatio={form.watch('CompletionRatio')}
              imageRatio={form.watch('ImageRatio')}
              audioRatio={form.watch('AudioRatio')}
              audioCompletionRatio={form.watch('AudioCompletionRatio')}
              billingMode={form.watch('BillingMode')}
              billingExpr={form.watch('BillingExpr')}
              candidateModelNames={
                isUnsetVariant ? enabledModelsQuery.data?.data : undefined
              }
              candidateModelsLoading={
                isUnsetVariant && enabledModelsQuery.isLoading
              }
              filterMode={isUnsetVariant ? 'unset' : 'all'}
              onSave={handleSave}
              isSaving={isSaving}
              onChange={(field, value) => {
                const fieldMap: Record<string, keyof ModelFormValues> = {
                  'billing_setting.billing_mode': 'BillingMode',
                  'billing_setting.billing_expr': 'BillingExpr',
                }
                const formField =
                  fieldMap[field] || (field as keyof ModelFormValues)
                handleFieldChange(formField, value)
              }}
            />

            {!isUnsetVariant && (
              <FormField
                control={form.control}
                name='ExposeRatioEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Expose ratio API')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Allow clients to query configured ratios via `/api/ratio`.'
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
            )}
          </div>
        ) : (
          <SettingsForm onSubmit={form.handleSubmit(reviewSave)}>
            <div className='grid min-w-0 gap-x-5 gap-y-8 lg:grid-cols-2 2xl:grid-cols-3'>
              {modelJsonFields.map((config) => (
                <ModelJsonTextareaField
                  key={config.name}
                  form={form}
                  name={config.name}
                  label={t(config.labelKey)}
                  description={t(config.descriptionKey)}
                />
              ))}
            </div>

            <FormField
              control={form.control}
              name='ExposeRatioEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Expose ratio API')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Allow clients to query configured ratios via `/api/ratio`.'
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
          </SettingsForm>
        )}
      </Form>
      <ConfirmDialog
        open={pendingSave !== null}
        onOpenChange={(open) => {
          if (!open && !confirming && !isSaving) setPendingSave(null)
        }}
        title={t('Review price changes')}
        desc={t('Review the current and new prices before saving.')}
        confirmText={t('Save model prices')}
        handleConfirm={() => void confirmSave()}
        isLoading={confirming || isSaving}
        className='sm:max-w-3xl'
      >
        {pendingSave && (
          <PriceChangeDiff
            changes={pendingSave.changes}
            unreadable={pendingSave.unreadable}
          />
        )}
        {pendingSave &&
          pendingSave.values.ExposeRatioEnabled !==
            pendingSave.exposedBefore && (
            <p>
              {t('Expose ratio API')}:{' '}
              {t(pendingSave.exposedBefore ? 'Enabled' : 'Disabled')} →{' '}
              {t(
                pendingSave.values.ExposeRatioEnabled ? 'Enabled' : 'Disabled'
              )}
            </p>
          )}
      </ConfirmDialog>
    </div>
  )
})

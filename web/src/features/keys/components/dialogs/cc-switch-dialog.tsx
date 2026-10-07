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
import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
  useComboboxAnchor,
} from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { useAuthStore } from '@/stores/auth-store'

import { getCCSwitchModels } from '../../lib/cc-switch-models'
import type { ApiKey } from '../../types'

const APP_CONFIGS = {
  claude: {
    label: 'Claude',
    defaultName: 'My Claude',
    modelFields: [
      { key: 'model', labelKey: 'Primary Model', required: true },
      { key: 'haikuModel', labelKey: 'Haiku Model', required: false },
      { key: 'sonnetModel', labelKey: 'Sonnet Model', required: false },
      { key: 'opusModel', labelKey: 'Opus Model', required: false },
    ],
  },
  codex: {
    label: 'Codex',
    defaultName: 'My Codex',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
  gemini: {
    label: 'Gemini',
    defaultName: 'My Gemini',
    modelFields: [{ key: 'model', labelKey: 'Primary Model', required: true }],
  },
} as const

type AppType = keyof typeof APP_CONFIGS

function getServerAddress(): string {
  try {
    const raw = localStorage.getItem('status')
    if (raw) {
      const status = JSON.parse(raw)
      if (status.server_address) return status.server_address
    }
  } catch {
    /* empty */
  }
  return window.location.origin
}

function buildCCSwitchURL(
  app: string,
  name: string,
  models: Record<string, string>,
  apiKey: string
): string {
  const serverAddress = getServerAddress()
  const endpoint = app === 'codex' ? serverAddress + '/v1' : serverAddress
  const params = new URLSearchParams()
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', name)
  params.set('endpoint', endpoint)
  params.set('apiKey', apiKey)
  for (const [k, v] of Object.entries(models)) {
    if (v) params.set(k, v)
  }
  params.set('homepage', serverAddress)
  params.set('enabled', 'true')
  return `ccswitch://v1/import?${params.toString()}`
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  token: ApiKey | null
}

function CCSwitchModelPicker(props: {
  id: string
  label: string
  options: string[]
  value: string
  onValueChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const anchor = useComboboxAnchor()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  return (
    <Combobox
      items={props.options}
      value={props.value || null}
      open={open}
      inputValue={open ? search : props.value}
      onInputValueChange={(value, details) => {
        if (details.reason === 'input-change') setSearch(value)
      }}
      onOpenChange={(nextOpen, details) => {
        setOpen(nextOpen)
        if (details.reason !== 'input-change') setSearch('')
      }}
      onValueChange={(value) => {
        if (value) props.onValueChange(value)
      }}
    >
      <div ref={anchor} className='relative min-w-0'>
        <ComboboxInput
          id={props.id}
          aria-label={props.label}
          placeholder={t('Select or enter model name')}
          showTrigger={false}
          className='w-full pr-8'
        />
        <ComboboxTrigger
          aria-label={props.label}
          className='absolute inset-y-0 right-0 flex w-8 items-center justify-center'
        />
      </div>
      <ComboboxContent anchor={anchor}>
        <ComboboxEmpty>{t('No models found')}</ComboboxEmpty>
        <ComboboxList>
          {(model: string) => (
            <ComboboxItem key={model} value={model}>
              {model}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const [app, setApp] = useState<AppType>('claude')
  const [name, setName] = useState<string>(APP_CONFIGS.claude.defaultName)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const userGroup = useAuthStore((state) => state.auth.user?.group)
  const requestContext = useMemo(
    () => ({
      identity: Symbol('cc-switch-models'),
      tokenKey: props.tokenKey,
      tokenId: props.token?.id,
      group: props.token?.group,
      autoGroups: props.token?.auto_groups,
      limitsEnabled: props.token?.model_limits_enabled,
      limits: props.token?.model_limits,
      userId,
      userGroup,
    }),
    [
      props.tokenKey,
      props.token?.id,
      props.token?.group,
      props.token?.auto_groups,
      props.token?.model_limits_enabled,
      props.token?.model_limits,
      userId,
      userGroup,
    ]
  )
  const requestIdentity = requestContext.identity
  const [result, setResult] = useState<{
    identity: symbol
    options: string[]
    status: 'loading' | 'ready' | 'error'
  } | null>(null)
  const [selection, setSelection] = useState<{
    identity: symbol
    models: Record<string, string>
  } | null>(null)
  const currentResult = result?.identity === requestIdentity ? result : null
  const modelOptions =
    props.open && currentResult?.status === 'ready' ? currentResult.options : []
  const models = selection?.identity === requestIdentity ? selection.models : {}

  useEffect(() => {
    if (!props.open) return
    setSelection(null)
    setApp('claude')
    setName(APP_CONFIGS.claude.defaultName)
    setResult({ identity: requestIdentity, options: [], status: 'loading' })
    if (!props.tokenKey || !props.token) return
    const controller = new AbortController()
    let active = true
    getCCSwitchModels(props.tokenKey, controller.signal)
      .then((options) => {
        if (active) {
          setResult({ identity: requestIdentity, options, status: 'ready' })
        }
      })
      .catch(() => {
        if (active) {
          setResult({ identity: requestIdentity, options: [], status: 'error' })
        }
      })
    return () => {
      active = false
      controller.abort()
    }
  }, [props.open, props.tokenKey, props.token, requestIdentity])

  const currentConfig = APP_CONFIGS[app]

  const handleAppChange = (val: string) => {
    const appVal = val as AppType
    setApp(appVal)
    setName(APP_CONFIGS[appVal].defaultName)
    setSelection(null)
  }

  const handleSubmit = () => {
    if (currentResult?.status !== 'ready') return
    if (
      !models.model ||
      Object.values(models).some(
        (model) => model && !modelOptions.includes(model)
      )
    ) {
      toast.warning(t('Please select a primary model'))
      return
    }
    const key = props.tokenKey.startsWith('sk-')
      ? props.tokenKey
      : `sk-${props.tokenKey}`
    const url = buildCCSwitchURL(app, name, models, key)
    window.open(url, '_blank')
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Import to CC Switch')}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={currentResult?.status !== 'ready'}
          >
            {t('Open CC Switch')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        {currentResult?.status === 'error' && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t(
                'Unable to load models for this API key. Check its permissions and availability.'
              )}
            </AlertDescription>
          </Alert>
        )}
        <div className='space-y-2'>
          <Label>{t('Application')}</Label>
          <RadioGroup
            value={app}
            onValueChange={handleAppChange}
            className='flex gap-4'
          >
            {(
              Object.entries(APP_CONFIGS) as [
                AppType,
                (typeof APP_CONFIGS)[AppType],
              ][]
            ).map(([key, cfg]) => (
              <div key={key} className='flex items-center gap-2'>
                <RadioGroupItem value={key} id={`app-${key}`} />
                <Label htmlFor={`app-${key}`} className='cursor-pointer'>
                  {cfg.label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='cc-switch-name'>{t('Name')}</Label>
          <Input
            id='cc-switch-name'
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={currentConfig.defaultName}
          />
        </div>

        {currentConfig.modelFields.map((field) => (
          <div key={field.key} className='space-y-2'>
            <Label htmlFor={`cc-switch-${field.key}`}>
              {t(field.labelKey)}
              {field.required && (
                <span className='text-destructive ml-0.5'>*</span>
              )}
            </Label>
            <CCSwitchModelPicker
              id={`cc-switch-${field.key}`}
              label={t(field.labelKey)}
              options={modelOptions}
              value={models[field.key] || ''}
              onValueChange={(v) =>
                setSelection((previous) => ({
                  identity: requestIdentity,
                  models: {
                    ...(previous?.identity === requestIdentity
                      ? previous.models
                      : {}),
                    [field.key]: v,
                  },
                }))
              }
            />
          </div>
        ))}
      </div>
    </Dialog>
  )
}

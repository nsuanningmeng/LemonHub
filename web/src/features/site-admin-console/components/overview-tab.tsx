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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { WalletLogsTable } from '@/components/wallet-logs-table'
import { formatMilliYuan } from '@/lib/wallet-log'

import { getDashboard, getWalletLogs, updateWarnThreshold } from '../api'
import { SUCCESS_MESSAGES } from '../constants'

const DASHBOARD_QUERY_KEY = ['site-admin-dashboard'] as const

export function OverviewTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: DASHBOARD_QUERY_KEY,
    queryFn: async () => {
      const res = await getDashboard()
      if (!res.success) {
        throw new Error(res.message || 'Failed to load')
      }
      return res.data ?? null
    },
  })

  const [draftThreshold, setDraftThreshold] = useState<string | null>(null)
  const [savingThreshold, setSavingThreshold] = useState(false)
  const [saveError, setSaveError] = useState('')
  const thresholdYuan =
    draftThreshold ??
    (data ? (data.wallet_warn_threshold / 1000).toFixed(3) : '')

  const handleSaveThreshold = async () => {
    const yuan = Number(thresholdYuan)
    if (!thresholdYuan.trim() || !Number.isFinite(yuan) || yuan < 0) {
      setSaveError(t('Enter a threshold of zero or more'))
      return
    }
    setSaveError('')
    setSavingThreshold(true)
    try {
      const result = await updateWarnThreshold(Math.round(yuan * 1000))
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.WARN_THRESHOLD_UPDATED))
        await queryClient.invalidateQueries({ queryKey: DASHBOARD_QUERY_KEY })
        setDraftThreshold(null)
      } else {
        setSaveError(result.message || t('Failed to save'))
      }
    } catch {
      setSaveError(t('Failed to save'))
    } finally {
      setSavingThreshold(false)
    }
  }

  if (isLoading) {
    return (
      <div className='flex items-center justify-center py-16'>
        <Spinner />
      </div>
    )
  }

  if (isError && !data) {
    return <ErrorState onRetry={() => void refetch()} />
  }

  if (!data) {
    return (
      <div className='text-muted-foreground py-16 text-center text-sm'>
        {t('No data')}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid items-start gap-5 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]'>
        {/* Wallet card */}
        <Card>
          <CardHeader>
            <CardTitle>{t('Current Balance')}</CardTitle>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <div className='text-4xl font-semibold tracking-tight tabular-nums'>
              ¥{formatMilliYuan(data.wallet_balance)}
            </div>
            <div className='flex flex-col gap-2'>
              <label
                htmlFor='site-wallet-threshold'
                className='text-muted-foreground text-xs'
              >
                {t('Wallet Warn Threshold')} <span>{t('CNY')}</span>
              </label>
              <div className='flex items-center gap-2'>
                <Input
                  id='site-wallet-threshold'
                  type='number'
                  step='0.001'
                  min='0'
                  value={thresholdYuan}
                  aria-describedby={saveError ? 'threshold-error' : undefined}
                  disabled={savingThreshold}
                  onChange={(e) => {
                    setDraftThreshold(e.target.value)
                    setSaveError('')
                  }}
                  className='max-w-40 min-w-0'
                />
                <Button
                  size='sm'
                  onClick={handleSaveThreshold}
                  disabled={savingThreshold}
                >
                  {savingThreshold ? t('Saving...') : t('Save')}
                </Button>
              </div>
              {saveError && (
                <p
                  id='threshold-error'
                  role='alert'
                  className='text-destructive text-sm'
                >
                  {saveError}
                </p>
              )}
            </div>
          </CardContent>
        </Card>

        {/* Branding overview card */}
        <Card>
          <CardHeader>
            <CardTitle>{t('Site Information')}</CardTitle>
            <CardDescription>{data.name}</CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-3 text-sm'>
            <InfoRow label={t('Status')}>
              <StatusBadge
                label={data.status === 1 ? t('Normal') : t('Disabled')}
                variant={data.status === 1 ? 'success' : 'neutral'}
                copyable={false}
              />
            </InfoRow>
            <InfoRow label={t('Discount Rate')}>
              <span className='font-mono'>
                {data.discount_rate === 10000
                  ? t('No Discount')
                  : `${(data.discount_rate / 100).toFixed(0)}%`}
              </span>
            </InfoRow>
            <InfoRow label={t('Domains')}>
              <span className='text-muted-foreground min-w-0 text-end wrap-anywhere'>
                {data.domains.length === 0 ? '-' : data.domains.join(', ')}
              </span>
            </InfoRow>
          </CardContent>
        </Card>
      </div>

      {/* Wallet ledger */}
      <Card>
        <CardHeader>
          <CardTitle>{t('Wallet Logs')}</CardTitle>
        </CardHeader>
        <CardContent>
          <WalletLogsTable
            queryKey='site-admin-wallet-logs'
            fetchLogs={async (params) => {
              const res = await getWalletLogs(params)
              if (!res.success) {
                throw new Error(res.message || t('Request failed'))
              }
              return {
                items: res.data?.items ?? [],
                total: res.data?.total ?? 0,
              }
            }}
          />
        </CardContent>
      </Card>
    </div>
  )
}

function InfoRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='flex items-center justify-between gap-3'>
      <span className='text-muted-foreground'>{label}</span>
      {children}
    </div>
  )
}

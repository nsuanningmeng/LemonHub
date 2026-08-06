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
import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { StatusBadge } from '@/components/status-badge'
import {
  WalletLogsTable,
  formatMilliYuan,
} from '@/components/wallet-logs-table'
import { getDashboard, getWalletLogs, updateWarnThreshold } from '../api'
import { SUCCESS_MESSAGES } from '../constants'

const DASHBOARD_QUERY_KEY = ['site-admin-dashboard'] as const

export function OverviewTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: DASHBOARD_QUERY_KEY,
    queryFn: async () => {
      const res = await getDashboard()
      return res.data ?? null
    },
  })

  const [thresholdYuan, setThresholdYuan] = useState('')
  const [savingThreshold, setSavingThreshold] = useState(false)

  useEffect(() => {
    if (data) {
      setThresholdYuan((data.wallet_warn_threshold / 1000).toFixed(3))
    }
  }, [data])

  const handleSaveThreshold = async () => {
    const yuan = parseFloat(thresholdYuan)
    if (!Number.isFinite(yuan) || yuan < 0) {
      toast.error(t('Amount must be greater than 0'))
      return
    }
    setSavingThreshold(true)
    try {
      const result = await updateWarnThreshold(Math.round(yuan * 1000))
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.WARN_THRESHOLD_UPDATED))
        queryClient.invalidateQueries({ queryKey: DASHBOARD_QUERY_KEY })
      }
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

  if (!data) {
    return (
      <div className='text-muted-foreground py-16 text-center text-sm'>
        {t('No data')}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid gap-4 md:grid-cols-2'>
        {/* Wallet card */}
        <Card>
          <CardHeader>
            <CardTitle>{t('Wallet')}</CardTitle>
            <CardDescription>{t('Current Balance')}</CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            <div className='font-mono text-3xl font-semibold'>
              ¥{formatMilliYuan(data.wallet_balance)}
            </div>
            <div className='flex flex-col gap-2'>
              <label className='text-muted-foreground text-xs'>
                {t('Wallet Warn Threshold')} ({t('CNY')})
              </label>
              <div className='flex items-center gap-2'>
                <Input
                  type='number'
                  step='0.001'
                  min='0'
                  value={thresholdYuan}
                  onChange={(e) => setThresholdYuan(e.target.value)}
                  className='w-[160px]'
                />
                <Button
                  size='sm'
                  onClick={handleSaveThreshold}
                  disabled={savingThreshold}
                >
                  {savingThreshold ? t('Saving...') : t('Save')}
                </Button>
              </div>
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
              <span className='text-muted-foreground max-w-[260px] truncate'>
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

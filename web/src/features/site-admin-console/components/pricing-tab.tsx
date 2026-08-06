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
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { getDashboard, updateModelPricing } from '../api'

const DASHBOARD_QUERY_KEY = ['site-admin-dashboard'] as const
const RETAIL_BASE = 10000

export function PricingTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rate, setRate] = useState<number>(RETAIL_BASE)
  const [submitting, setSubmitting] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: DASHBOARD_QUERY_KEY,
    queryFn: async () => {
      const res = await getDashboard()
      return res.data ?? null
    },
  })

  useEffect(() => {
    if (data?.model_price_rate) setRate(data.model_price_rate)
  }, [data])

  if (isLoading) {
    return (
      <div className='flex items-center justify-center py-16'>
        <Spinner />
      </div>
    )
  }

  const cap = data?.model_price_rate_max ?? 0
  const multiplier = (rate / RETAIL_BASE).toFixed(2)

  const onSubmit = async () => {
    if (rate < RETAIL_BASE) {
      toast.error(t('Markup cannot be below 10000 (the platform retail price).'))
      return
    }
    if (cap > 0 && rate > cap) {
      toast.error(t('Markup exceeds the cap set by the platform.'))
      return
    }
    setSubmitting(true)
    try {
      const res = await updateModelPricing(rate)
      if (res.success) {
        toast.success(t('Model pricing updated'))
        queryClient.invalidateQueries({ queryKey: DASHBOARD_QUERY_KEY })
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card className='max-w-2xl'>
      <CardHeader>
        <CardTitle>{t('Model Pricing')}</CardTitle>
      </CardHeader>
      <CardContent className='flex flex-col gap-5'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Set how much your users pay per model call relative to the platform retail price. 10000 = same as the platform (default); you may only mark UP. This does not change how your procurement wallet is debited.'
          )}
        </p>
        <div className='flex flex-col gap-2'>
          <div className='text-sm font-medium'>
            {t('Model Price Markup (basis of 10000)')}
          </div>
          <Input
            type='number'
            min={RETAIL_BASE}
            max={cap > 0 ? cap : undefined}
            value={rate}
            onChange={(e) =>
              setRate(parseInt(e.target.value, 10) || RETAIL_BASE)
            }
          />
          <p className='text-muted-foreground text-xs'>
            {t('Current')}: {multiplier}× ({t('platform retail')} = 1.00×)
            {cap > 0
              ? ` · ${t('Max allowed')}: ${(cap / RETAIL_BASE).toFixed(2)}× (${cap})`
              : ` · ${t('No upper cap')}`}
          </p>
        </div>
        <div className='flex justify-end'>
          <Button onClick={onSubmit} disabled={submitting}>
            {submitting ? t('Saving...') : t('Save changes')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

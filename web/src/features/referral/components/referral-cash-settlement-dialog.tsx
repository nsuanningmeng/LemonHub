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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import {
  formatQuota,
  formatTimestampToDate,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'

import {
  useReferralAdminCashPayouts,
  useRecordReferralAdminCashPayout,
} from '../hooks/use-referral-admin-cash-payouts'
import type { AffAdminLeaderboardItem } from '../types'

interface ReferralCashSettlementDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The promoter being settled; null keeps the dialog (and history query) idle. */
  item: AffAdminLeaderboardItem | null
}

/**
 * Admin dialog to record an off-platform cash settlement for a cash-settled promoter.
 *
 * Units: outstanding/total/paid are quota units rendered via `formatQuota`. The amount input
 * follows the same "$ ↔ quota" convention used by the user "Adjust Quota" dialog — the admin
 * types an amount in the configured display currency (or raw tokens) and it is converted to
 * quota units with `parseQuotaFromDollars` before being submitted. The input is prefilled with
 * the outstanding balance, and amounts above the outstanding balance are blocked client-side
 * (mirroring the server guard).
 */
export function ReferralCashSettlementDialog(
  props: ReferralCashSettlementDialogProps
) {
  const { t } = useTranslation()
  const { item, open, onOpenChange } = props

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const outstanding = item?.cash_commission_owed ?? 0
  const totalCommission = item?.cash_commission_total ?? 0
  const paid = item?.cash_commission_paid ?? 0

  const inviterId = open && item ? item.inviter_id : null
  const {
    data: payouts,
    isLoading: historyLoading,
    isError: historyError,
  } = useReferralAdminCashPayouts(inviterId)
  const recordMutation = useRecordReferralAdminCashPayout()

  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')

  // Convert quota units to a clean display-amount string for prefilling the input.
  const toDisplayAmount = (units: number) =>
    tokensOnly
      ? String(units)
      : String(Number(quotaUnitsToDollars(units).toFixed(6)))

  // Prefill with the outstanding balance whenever the dialog opens for a promoter.
  useEffect(() => {
    if (open && item) {
      const units = item.cash_commission_owed ?? 0
      setAmount(
        tokensOnly
          ? String(units)
          : String(Number(quotaUnitsToDollars(units).toFixed(6)))
      )
      setNote('')
    }
  }, [open, item, tokensOnly])

  const quotaAmount = parseQuotaFromDollars(parseFloat(amount) || 0)
  const exceedsOutstanding = quotaAmount > outstanding
  const submitDisabled =
    recordMutation.isPending || quotaAmount <= 0 || exceedsOutstanding

  const handleSubmit = () => {
    if (!item) return
    if (quotaAmount <= 0) return
    if (exceedsOutstanding) {
      toast.error(t('Amount exceeds outstanding'))
      return
    }

    recordMutation.mutate(
      { inviter_id: item.inviter_id, amount: quotaAmount, note: note.trim() },
      {
        onSuccess: (res) => {
          if (res.success) {
            toast.success(t('Cash settlement recorded'))
            setAmount('')
            setNote('')
            onOpenChange(false)
          }
          // Business failures (e.g. over-amount) are surfaced by the global API
          // interceptor using the server message, so no extra toast here.
        },
      }
    )
  }

  const items = payouts ?? []

  let history: React.ReactNode
  if (historyLoading) {
    history = (
      <div className='flex flex-col gap-2'>
        <Skeleton className='h-8 w-full' />
        <Skeleton className='h-8 w-full' />
        <Skeleton className='h-8 w-full' />
      </div>
    )
  } else if (historyError) {
    history = (
      <p className='text-muted-foreground text-sm'>{t('Failed to load')}</p>
    )
  } else if (items.length === 0) {
    history = (
      <p className='text-muted-foreground text-sm'>
        {t('No settlement records yet')}
      </p>
    )
  } else {
    history = (
      <ul className='divide-border/60 divide-y'>
        {items.map((payout) => (
          <li
            key={payout.id}
            className='flex items-start justify-between gap-3 py-2'
          >
            <div className='min-w-0'>
              <span className='font-medium tabular-nums'>
                {formatQuota(payout.amount)}
              </span>
              {payout.note && (
                <p className='text-muted-foreground truncate text-xs'>
                  {payout.note}
                </p>
              )}
            </div>
            <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
              {formatTimestampToDate(payout.created_at)}
            </span>
          </li>
        ))}
      </ul>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Record cash settlement')}
      description={
        item
          ? t('Record an off-platform cash payment for this promoter.')
          : undefined
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={submitDisabled}>
            {recordMutation.isPending ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        {/* Balance summary */}
        <div className='bg-muted/40 grid grid-cols-3 gap-2 rounded-md p-3 text-center'>
          <div>
            <p className='text-muted-foreground text-xs'>{t('Outstanding')}</p>
            <p className='font-semibold tabular-nums'>
              {formatQuota(outstanding)}
            </p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>{t('Total')}</p>
            <p className='tabular-nums'>{formatQuota(totalCommission)}</p>
          </div>
          <div>
            <p className='text-muted-foreground text-xs'>{t('Paid')}</p>
            <p className='tabular-nums'>{formatQuota(paid)}</p>
          </div>
        </div>

        {/* Amount input */}
        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <Label htmlFor='cash-settlement-amount'>
              {t('Settlement amount')} ({currencyLabel})
            </Label>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setAmount(toDisplayAmount(outstanding))}
            >
              {t('Settle in full')}
            </Button>
          </div>
          <Input
            id='cash-settlement-amount'
            type='number'
            step={tokensOnly ? 1 : 0.000001}
            min={0}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder={
              tokensOnly
                ? t('Enter amount in tokens')
                : t('Enter amount in {{currency}}', { currency: currencyLabel })
            }
            aria-invalid={exceedsOutstanding}
          />
          {exceedsOutstanding && (
            <p className='text-destructive text-xs'>
              {t('Amount exceeds outstanding')}
            </p>
          )}
        </div>

        {/* Optional note */}
        <div className='space-y-2'>
          <Label htmlFor='cash-settlement-note'>{t('Note (optional)')}</Label>
          <Textarea
            id='cash-settlement-note'
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
          />
        </div>

        {/* Settlement history */}
        <div className='space-y-2'>
          <Label>{t('Settlement history')}</Label>
          {history}
        </div>
      </div>
    </Dialog>
  )
}

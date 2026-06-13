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
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import {
  WalletLogsTable,
  formatMilliYuan,
} from '@/components/wallet-logs-table'
import { adjustSiteWallet, getSiteWalletLogs, rechargeSiteWallet } from '../api'
import {
  getAdjustFormSchema,
  getRechargeFormSchema,
  type AdjustFormValues,
  type RechargeFormValues,
  ADJUST_FORM_DEFAULT_VALUES,
  RECHARGE_FORM_DEFAULT_VALUES,
  yuanToMilli,
} from '../lib'
import { useSubSite } from './sub-site-provider'

export function SubSiteWalletDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useSubSite()
  const isOpen = open === 'wallet'

  const [balance, setBalance] = useState<number>(0)
  const [logsRefresh, setLogsRefresh] = useState(0)
  const [submitting, setSubmitting] = useState<'recharge' | 'adjust' | null>(
    null
  )

  const rechargeForm = useForm<RechargeFormValues>({
    resolver: zodResolver(getRechargeFormSchema(t)),
    defaultValues: RECHARGE_FORM_DEFAULT_VALUES,
  })

  const adjustForm = useForm<AdjustFormValues>({
    resolver: zodResolver(getAdjustFormSchema(t)),
    defaultValues: ADJUST_FORM_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (isOpen && currentRow) {
      setBalance(currentRow.wallet_balance)
      rechargeForm.reset(RECHARGE_FORM_DEFAULT_VALUES)
      adjustForm.reset(ADJUST_FORM_DEFAULT_VALUES)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, currentRow])

  if (!currentRow) return null
  const siteId = currentRow.id

  const onRecharge = async (data: RechargeFormValues) => {
    setSubmitting('recharge')
    try {
      const result = await rechargeSiteWallet(siteId, {
        amount: yuanToMilli(data.amount_yuan),
        remark: data.remark || '',
      })
      if (result.success) {
        toast.success(t('Recharge successful'))
        if (result.data) setBalance(result.data.wallet_balance)
        rechargeForm.reset(RECHARGE_FORM_DEFAULT_VALUES)
        setLogsRefresh((v) => v + 1)
        triggerRefresh()
      }
    } finally {
      setSubmitting(null)
    }
  }

  const onAdjust = async (data: AdjustFormValues) => {
    setSubmitting('adjust')
    try {
      const result = await adjustSiteWallet(siteId, {
        amount: yuanToMilli(data.amount_yuan),
        remark: data.remark,
      })
      if (result.success) {
        toast.success(t('Adjustment successful'))
        if (result.data) setBalance(result.data.wallet_balance)
        adjustForm.reset(ADJUST_FORM_DEFAULT_VALUES)
        setLogsRefresh((v) => v + 1)
        triggerRefresh()
      }
    } finally {
      setSubmitting(null)
    }
  }

  return (
    <Sheet open={isOpen} onOpenChange={(v) => !v && setOpen(null)}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[720px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Wallet Management')}</SheetTitle>
          <SheetDescription>
            {currentRow.name} (ID: {siteId})
          </SheetDescription>
        </SheetHeader>

        <div className={sideDrawerFormClassName()}>
          {/* Balance summary */}
          <SideDrawerSection>
            <div className='flex flex-wrap items-end justify-between gap-4'>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Current Balance')}
                </div>
                <div className='font-mono text-2xl font-semibold'>
                  ¥{formatMilliYuan(balance)}
                </div>
              </div>
              <div className='text-right'>
                <div className='text-muted-foreground text-xs'>
                  {t('Wallet Warn Threshold')}
                </div>
                <div className='font-mono text-sm'>
                  ¥{formatMilliYuan(currentRow.wallet_warn_threshold)}
                </div>
              </div>
            </div>
          </SideDrawerSection>

          {/* Recharge form */}
          <SideDrawerSection>
            <Form {...rechargeForm}>
              <form
                onSubmit={rechargeForm.handleSubmit(onRecharge)}
                className='flex flex-col gap-4'
              >
                <h3 className='text-sm font-semibold'>{t('Recharge')}</h3>
                <FormField
                  control={rechargeForm.control}
                  name='amount_yuan'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Recharge Amount (CNY)')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step='0.001'
                          min='0'
                          placeholder='0.000'
                          onChange={(e) =>
                            field.onChange(parseFloat(e.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={rechargeForm.control}
                  name='remark'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Remark')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Enter remark (optional)')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className='flex justify-end'>
                  <Button type='submit' disabled={submitting === 'recharge'}>
                    {submitting === 'recharge'
                      ? t('Saving...')
                      : t('Submit Recharge')}
                  </Button>
                </div>
              </form>
            </Form>
          </SideDrawerSection>

          {/* Manual adjustment form */}
          <SideDrawerSection>
            <Form {...adjustForm}>
              <form
                onSubmit={adjustForm.handleSubmit(onAdjust)}
                className='flex flex-col gap-4'
              >
                <h3 className='text-sm font-semibold'>
                  {t('Manual Adjustment')}
                </h3>
                <FormField
                  control={adjustForm.control}
                  name='amount_yuan'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Adjustment Amount (CNY)')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step='0.001'
                          placeholder='0.000'
                          onChange={(e) =>
                            field.onChange(parseFloat(e.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Use a negative value to deduct from the balance.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={adjustForm.control}
                  name='remark'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Remark')}
                        <span className='text-destructive ml-1'>*</span>
                      </FormLabel>
                      <FormControl>
                        <Textarea
                          {...field}
                          rows={2}
                          placeholder={t('Enter remark (required)')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className='flex justify-end'>
                  <Button
                    type='submit'
                    variant='outline'
                    disabled={submitting === 'adjust'}
                  >
                    {submitting === 'adjust'
                      ? t('Saving...')
                      : t('Apply Adjustment')}
                  </Button>
                </div>
              </form>
            </Form>
          </SideDrawerSection>

          {/* Wallet ledger */}
          <SideDrawerSection>
            <h3 className='mb-2 text-sm font-semibold'>{t('Wallet Logs')}</h3>
            <WalletLogsTable
              queryKey={`sub-site-wallet-logs-${siteId}`}
              refreshKey={logsRefresh}
              fetchLogs={async (params) => {
                const res = await getSiteWalletLogs(siteId, params)
                return {
                  items: res.data?.items ?? [],
                  total: res.data?.total ?? 0,
                }
              }}
            />
          </SideDrawerSection>
        </div>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { Spinner } from '@/components/ui/spinner'
import { getPayConfig, updatePayConfig } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import {
  getPayConfigFormSchema,
  type PayConfigFormValues,
  PAY_CONFIG_FORM_DEFAULT_VALUES,
  transformPayConfigFormToPayload,
  transformPayConfigToFormValues,
} from '../lib'

const PAY_CONFIG_QUERY_KEY = ['site-admin-pay-config'] as const

export function PayConfigTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: PAY_CONFIG_QUERY_KEY,
    queryFn: async () => {
      const res = await getPayConfig()
      return res.data ?? null
    },
  })

  const form = useForm<PayConfigFormValues>({
    resolver: zodResolver(getPayConfigFormSchema(t)),
    defaultValues: PAY_CONFIG_FORM_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (data) {
      form.reset(transformPayConfigToFormValues(data))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const onSubmit = async (values: PayConfigFormValues) => {
    setIsSubmitting(true)
    try {
      const result = await updatePayConfig(
        transformPayConfigFormToPayload(values)
      )
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.PAY_CONFIG_UPDATED))
        queryClient.invalidateQueries({ queryKey: PAY_CONFIG_QUERY_KEY })
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  if (isLoading) {
    return (
      <div className='flex items-center justify-center py-16'>
        <Spinner />
      </div>
    )
  }

  return (
    <Card className='max-w-2xl'>
      <CardHeader>
        <CardTitle>{t('Payment Settings')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className='flex flex-col gap-5'
          >
            <FormField
              control={form.control}
              name='epay_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Merchant ID')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder='1001' />
                  </FormControl>
                  <FormDescription>
                    {t('Merchant ID help text')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='epay_key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Merchant Key')}</FormLabel>
                  <FormControl>
                    <Input {...field} type='password' placeholder='••••••••' />
                  </FormControl>
                  <FormDescription>
                    {t('Merchant Key help text')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='pay_address'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Payment Gateway URL')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder='https://pay.example.com'
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Payment Gateway URL help text')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='pay_methods'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Payment Methods')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder='alipay,wxpay' />
                  </FormControl>
                  <FormDescription>
                    {t('Payment Methods help text')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className='flex justify-end'>
              <Button type='submit' disabled={isSubmitting}>
                {isSubmitting ? t('Saving...') : t('Save changes')}
              </Button>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}

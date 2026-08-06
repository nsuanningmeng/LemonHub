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
import { Textarea } from '@/components/ui/textarea'
import { getDashboard, updateBranding } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import {
  getBrandingFormSchema,
  type BrandingFormValues,
  BRANDING_FORM_DEFAULT_VALUES,
  transformBrandingFormToPayload,
} from '../lib'

const DASHBOARD_QUERY_KEY = ['site-admin-dashboard'] as const

export function BrandingTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: DASHBOARD_QUERY_KEY,
    queryFn: async () => {
      const res = await getDashboard()
      return res.data ?? null
    },
  })

  const form = useForm<BrandingFormValues>({
    resolver: zodResolver(getBrandingFormSchema(t)),
    defaultValues: BRANDING_FORM_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (data) {
      form.reset({
        name: data.name,
        logo: data.logo || '',
        notice: data.notice || '',
        footer: data.footer || '',
        home_badge: data.home_badge || '',
        home_title_line1: data.home_title_line1 || '',
        home_title_line2: data.home_title_line2 || '',
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const onSubmit = async (values: BrandingFormValues) => {
    setIsSubmitting(true)
    try {
      const result = await updateBranding(
        transformBrandingFormToPayload(values)
      )
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.BRANDING_UPDATED))
        queryClient.invalidateQueries({ queryKey: DASHBOARD_QUERY_KEY })
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
        <CardTitle>{t('Branding Settings')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className='flex flex-col gap-5'
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t('Enter sub-site name')} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='logo'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Logo URL')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder='https://example.com/logo.png'
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='notice'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Notice')}</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={3}
                      placeholder={t('Homepage notice (supports Markdown)')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Homepage notice (supports Markdown)')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='footer'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Footer')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder={t('Footer text or HTML')} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='home_badge'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Home Hero Badge')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('Leave empty to use the site title')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Small pill shown above the homepage hero title.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='home_title_line1'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Home Hero Title Line 1')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('Leave empty to use the default copy')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='home_title_line2'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Home Hero Title Line 2')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('Leave empty to use the default copy')}
                    />
                  </FormControl>
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

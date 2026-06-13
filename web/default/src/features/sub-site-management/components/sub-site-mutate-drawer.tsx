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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { createSite, getSite, updateSite } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import {
  getSiteFormSchema,
  type SiteFormValues,
  SITE_FORM_DEFAULT_VALUES,
  transformFormToPayload,
  transformSiteToForm,
} from '../lib'
import { type Site } from '../types'
import { useSubSite } from './sub-site-provider'

type SubSiteMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Site
}

export function SubSiteMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: SubSiteMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useSubSite()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [showPayConfig, setShowPayConfig] = useState(false)

  const form = useForm<SiteFormValues>({
    resolver: zodResolver(getSiteFormSchema(t)),
    defaultValues: SITE_FORM_DEFAULT_VALUES,
  })

  // Load existing data when updating
  useEffect(() => {
    if (open && isUpdate && currentRow) {
      // Fetch fresh data for update
      getSite(currentRow.id).then((result) => {
        if (result.success && result.data) {
          form.reset(transformSiteToForm(result.data))
        }
      })
    } else if (open && !isUpdate) {
      // Reset to defaults for create
      form.reset(SITE_FORM_DEFAULT_VALUES)
    }
  }, [open, isUpdate, currentRow, form])

  const onSubmit = async (data: SiteFormValues) => {
    setIsSubmitting(true)
    try {
      const payload = transformFormToPayload(data)

      if (isUpdate && currentRow) {
        const result = await updateSite({ ...payload, id: currentRow.id })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.SITE_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t('Failed to update sub-site'))
        }
      } else {
        const result = await createSite(payload)
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.SITE_CREATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t('Failed to create sub-site'))
        }
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  const statusItems = [
    { value: '1', label: t('Normal') },
    { value: '2', label: t('Disabled') },
  ]

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
          setShowPayConfig(false)
        }
      }}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[600px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update Sub-site') : t('Create Sub-site')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the sub-site by providing necessary info.')
              : t('Add a new sub-site by providing necessary info.')}{' '}
            {t('Click save when you&apos;re done.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='sub-site-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            <SideDrawerSection>
              {/* Name */}
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Enter sub-site name')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Domains */}
              <FormField
                control={form.control}
                name='domains_text'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Domains')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        placeholder={'example.com\napp.example.com'}
                        rows={3}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'One domain per line. At least one domain is required.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Owner Username */}
              <FormField
                control={form.control}
                name='owner_username'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Owner Username')}
                      {!isUpdate && (
                        <span className='text-destructive ml-1'>*</span>
                      )}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Enter owner username')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'The username of the account that will own this sub-site.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Logo URL */}
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

              {/* Status */}
              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Status')}</FormLabel>
                    <Select
                      items={statusItems}
                      value={String(field.value)}
                      onValueChange={(v) =>
                        v !== null && field.onChange(parseInt(v, 10))
                      }
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='1'>{t('Normal')}</SelectItem>
                          <SelectItem value='2'>{t('Disabled')}</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Discount Rate */}
              <FormField
                control={form.control}
                name='discount_rate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Discount Rate')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min='0'
                        max='10000'
                        placeholder='10000'
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value, 10) || 10000)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('10000=原价, 7000=7折. Basis of 10000.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Wallet Warn Threshold */}
              <FormField
                control={form.control}
                name='wallet_warn_threshold'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Wallet Warn Threshold')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min='0'
                        placeholder='0'
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value, 10) || 0)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Unit: 厘 (0.001 CNY). 0 means no warning.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Notice */}
              <FormField
                control={form.control}
                name='notice'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Notice')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        placeholder={t('Homepage notice (supports Markdown)')}
                        rows={3}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Footer */}
              <FormField
                control={form.control}
                name='footer'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Footer')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Footer text or HTML')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Pay Config (advanced, collapsed) */}
              <div>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  className='text-muted-foreground px-0 text-xs'
                  onClick={() => setShowPayConfig((v) => !v)}
                >
                  {showPayConfig ? t('Hide Advanced') : t('Show Advanced')}
                </Button>
                {showPayConfig && (
                  <FormField
                    control={form.control}
                    name='pay_config'
                    render={({ field }) => (
                      <FormItem className='mt-2'>
                        <FormLabel>{t('Pay Config')}</FormLabel>
                        <FormControl>
                          <Textarea
                            {...field}
                            placeholder='{"key": "value"}'
                            rows={4}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Payment configuration JSON (Phase 4).')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}
              </div>
            </SideDrawerSection>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button form='sub-site-form' type='submit' disabled={isSubmitting}>
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

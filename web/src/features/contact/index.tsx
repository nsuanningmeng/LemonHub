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
import { MailQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Markdown } from '@/components/ui/markdown'
import { Skeleton } from '@/components/ui/skeleton'
import { AnimateInView } from '@/components/animate-in-view'
import { Footer } from '@/components/layout/components/footer'
import { PublicLayout } from '@/components/layout'
import { getContactContent } from './api'

function isValidUrl(value: string) {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function isLikelyHtml(value: string) {
  return /<\/?[a-z][\s\S]*>/i.test(value)
}

// Default page shown when the admin has not configured any contact content in
// System Settings → Site → Contact. It intentionally shows NO concrete contact
// details (no email / IM handles) so a fresh deployment never publishes
// placeholder or fake support channels. The real contact page is authored by
// the admin via the Contact content field (HTML / URL / Markdown).
function DefaultContactPage() {
  const { t } = useTranslation()
  return (
    <main className='overflow-x-hidden'>
      <section className='relative z-10 overflow-hidden px-6 pt-24 pb-24 md:pt-32 md:pb-32'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 -z-10 opacity-25 dark:opacity-[0.12]'
          style={{
            background:
              'radial-gradient(ellipse 60% 50% at 50% 0%, oklch(0.72 0.18 250 / 70%) 0%, transparent 70%)',
          }}
        />
        <div
          aria-hidden
          className='absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_20%,black_20%,transparent_100%)] bg-[size:4rem_4rem] opacity-[0.08]'
        />

        <div className='mx-auto max-w-2xl'>
          <AnimateInView className='text-center'>
            <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
              {t('Get in Touch')}
            </p>
            <h1 className='text-[clamp(2rem,4vw,2.75rem)] leading-[1.15] font-bold tracking-tight'>
              {t('Contact Us')}
            </h1>
          </AnimateInView>

          <AnimateInView
            delay={120}
            animation='fade-up'
            className='border-border/50 bg-card mx-auto mt-12 flex max-w-lg flex-col items-center rounded-2xl border p-10 text-center'
          >
            <span className='border-border/50 bg-muted/30 text-muted-foreground mb-5 flex size-14 items-center justify-center rounded-2xl border'>
              <MailQuestion className='size-6' strokeWidth={1.5} />
            </span>
            <h2 className='mb-2 text-lg font-semibold'>
              {t("Contact details haven't been set up yet.")}
            </h2>
            <p className='text-muted-foreground text-sm leading-relaxed'>
              {t(
                'The administrator can configure the contact content in System Settings → Site Information → Contact.'
              )}
            </p>
          </AnimateInView>
        </div>
      </section>
      <Footer />
    </main>
  )
}

export function Contact() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['contact-content'],
    queryFn: getContactContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isValidUrl(rawContent)
  const isHtml = hasContent && !isUrl && isLikelyHtml(rawContent)

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  // Admin-configured content (set in System Settings → Site → Contact).
  // Mirrors the About page behaviour (URL → iframe, HTML, or Markdown).
  if (hasContent) {
    if (isUrl) {
      return (
        <PublicLayout showMainContainer={false}>
          <iframe
            src={rawContent}
            className='h-[calc(100vh-3.5rem)] w-full border-0'
            title={t('Contact Us')}
            referrerPolicy='no-referrer'
            sandbox='allow-scripts allow-forms allow-popups allow-same-origin'
          />
        </PublicLayout>
      )
    }
    return (
      <PublicLayout>
        <div className='mx-auto max-w-6xl px-4 py-8'>
          {isHtml ? (
            <div
              className='prose prose-neutral dark:prose-invert max-w-none'
              dangerouslySetInnerHTML={{ __html: rawContent }}
            />
          ) : (
            <Markdown className='prose-neutral dark:prose-invert max-w-none'>
              {rawContent}
            </Markdown>
          )}
        </div>
      </PublicLayout>
    )
  }

  // Default state — no fake contact data published.
  return (
    <PublicLayout showMainContainer={false}>
      <DefaultContactPage />
    </PublicLayout>
  )
}

export default Contact

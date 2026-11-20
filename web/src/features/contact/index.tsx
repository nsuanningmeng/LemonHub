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

import { HtmlContent } from '@/components/html-content'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { RichContent } from '@/components/rich-content'
import { Markdown } from '@/components/ui/markdown'
import {
  DocumentLayout,
  DocumentLoading,
  DocumentTitle,
} from '@/features/public-documents/document-layout'

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

function isHtmlDocument(value: string): boolean {
  return /^\s*(?:<!--[\s\S]*?-->\s*)*(?:<!doctype\s+html\b|<(?:html|head|body)(?:\s|>))/i.test(
    value
  )
}

// Default page shown when the admin has not configured any contact content in
// System Settings → Site → Contact. It intentionally shows NO concrete contact
// details (no email / IM handles) so a fresh deployment never publishes
// placeholder or fake support channels. The real contact page is authored by
// the admin via the Contact content field (HTML / URL / Markdown).
function DefaultContactPage() {
  const { t } = useTranslation()
  return (
    <div className='flex min-h-svh flex-col'>
      <main className='mx-auto w-full max-w-[76rem] flex-1 px-5 pt-28 pb-16 sm:px-8 sm:pt-36'>
        <DocumentTitle>{t('Contact Us')}</DocumentTitle>
        <div className='flex max-w-2xl items-start gap-4'>
          <MailQuestion
            aria-hidden='true'
            className='text-primary mt-0.5 size-6 shrink-0'
            strokeWidth={1.65}
          />
          <div>
            <h2 className='text-xl font-semibold tracking-tight'>
              {t("Contact details haven't been set up yet.")}
            </h2>
            <p className='text-muted-foreground mt-2 text-base leading-relaxed'>
              {t(
                'The administrator can configure the contact content in System Settings → Site Information → Contact.'
              )}
            </p>
          </div>
        </div>
      </main>
      <Footer />
    </div>
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
    return <DocumentLoading />
  }

  // Admin-configured content (set in System Settings → Site → Contact).
  // Mirrors the About page behaviour (URL → iframe, HTML, or Markdown).
  if (hasContent) {
    if (isUrl) {
      return (
        <PublicLayout showMainContainer={false}>
          <main className='pt-16'>
            <iframe
              src={rawContent}
              className='block h-[calc(100svh-4rem)] w-full border-0'
              title={t('Contact Us')}
              referrerPolicy='no-referrer'
              sandbox='allow-scripts allow-forms allow-popups'
            />
          </main>
        </PublicLayout>
      )
    }
    if (isHtml && isHtmlDocument(rawContent)) {
      return (
        <PublicLayout showMainContainer={false}>
          <main className='pt-16'>
            <HtmlContent
              variant='document'
              content={rawContent}
              title={t('Contact Us')}
              className='h-[calc(100svh-4rem)]'
            />
          </main>
        </PublicLayout>
      )
    }
    return (
      <DocumentLayout>
        {isHtml ? (
          <RichContent
            mode='html'
            content={rawContent}
            className='prose prose-neutral dark:prose-invert max-w-none'
          />
        ) : (
          <Markdown className='prose-neutral dark:prose-invert max-w-none'>
            {rawContent}
          </Markdown>
        )}
      </DocumentLayout>
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

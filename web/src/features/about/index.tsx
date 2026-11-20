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
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import {
  DocumentLayout,
  DocumentLoading,
  DocumentTitle,
} from '@/features/public-documents/document-layout'
import { useIframePreferences } from '@/hooks/use-iframe-preferences'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'

import { getAboutContent } from './api'

function EmptyAboutState() {
  const { t } = useTranslation()
  const currentYear = new Date().getFullYear()

  return (
    <DocumentLayout>
      <DocumentTitle>{t('About')}</DocumentTitle>
      <div className='space-y-3'>
        <h2 className='text-xl font-semibold'>{t('No About Content Set')}</h2>
        <p className='text-muted-foreground max-w-prose leading-relaxed'>
          {t(
            'The administrator has not configured any about content yet. You can set it in the settings page, supporting HTML or URL.'
          )}
        </p>
      </div>
      <div className='text-muted-foreground [&_a]:decoration-border mt-10 space-y-3 border-t pt-6 text-sm leading-relaxed [overflow-wrap:anywhere] [&_a]:underline [&_a]:underline-offset-4 hover:[&_a]:decoration-current'>
        <p>
          {t('New API Project Repository:')}{' '}
          <a
            href='https://github.com/QuantumNous/new-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('https://github.com/QuantumNous/new-api')}
          </a>
        </p>
        <p className='text-muted-foreground'>
          <a
            href='https://github.com/QuantumNous/new-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('NewAPI')}
          </a>{' '}
          © {currentYear}{' '}
          <a
            href='https://github.com/QuantumNous'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('QuantumNous')}
          </a>{' '}
          {t('| Based on')}{' '}
          <a
            href='https://github.com/songquanpeng/one-api'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('One API')}
          </a>{' '}
          © 2023{' '}
          <a
            href='https://github.com/songquanpeng'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('JustSong')}
          </a>
        </p>
        <p className='text-muted-foreground'>
          {t('This project must be used in compliance with the')}{' '}
          <a
            href='https://github.com/QuantumNous/new-api/blob/main/LICENSE'
            target='_blank'
            rel='noopener noreferrer'
            className='text-primary hover:underline'
          >
            {t('AGPL v3.0 License')}
          </a>
          .
        </p>
      </div>
    </DocumentLayout>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['about-content'],
    queryFn: getAboutContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)
  const { iframeRef, syncPreferences } = useIframePreferences(
    isUrl ? rawContent : null
  )

  if (isLoading) {
    return <DocumentLoading />
  }

  if (!hasContent) {
    return <EmptyAboutState />
  }

  if (isUrl) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='pt-16'>
          <iframe
            ref={iframeRef}
            src={rawContent}
            onLoad={syncPreferences}
            className='block h-[calc(100svh-4rem)] w-full border-0'
            title={t('About')}
            sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
          />
        </main>
      </PublicLayout>
    )
  }

  if (contentIsHtml) {
    return (
      <PublicLayout showMainContainer={false}>
        <RichContent
          mode='html'
          htmlVariant='isolated'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </PublicLayout>
    )
  }

  return (
    <DocumentLayout>
      <RichContent
        mode='markdown'
        content={rawContent}
        className='prose-neutral dark:prose-invert max-w-none'
      />
    </DocumentLayout>
  )
}

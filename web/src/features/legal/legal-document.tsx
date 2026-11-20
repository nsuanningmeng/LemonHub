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
import { ArrowUpRight, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { buttonVariants } from '@/components/ui/button'
import {
  DocumentLayout,
  DocumentLoading,
} from '@/features/public-documents/document-layout'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'

import type { LegalDocumentResponse } from './types'

type LegalDocumentProps = {
  title: string
  queryKey: string
  fetchDocument: () => Promise<LegalDocumentResponse>
  emptyMessage: string
}

export function LegalDocument({
  title,
  queryKey,
  fetchDocument,
  emptyMessage,
}: LegalDocumentProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: [queryKey],
    queryFn: fetchDocument,
    staleTime: 10 * 60 * 1000,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)
  const success = data?.success ?? false

  if (isLoading) {
    return <DocumentLoading />
  }

  if (!success || !hasContent || isUrl) {
    return (
      <DocumentLayout title={title}>
        <div className='flex items-start gap-4'>
          <FileText
            aria-hidden='true'
            className='text-muted-foreground mt-1 size-6 shrink-0'
          />
          <div>
            <p className='text-muted-foreground leading-relaxed'>
              {success && isUrl
                ? t('This document is published on an external site.')
                : data?.message || emptyMessage}
            </p>
            {success && isUrl && (
              <a
                href={rawContent}
                target='_blank'
                rel='noopener noreferrer'
                className={buttonVariants({ className: 'mt-5' })}
              >
                {t('View document')}
                <ArrowUpRight aria-hidden='true' />
              </a>
            )}
          </div>
        </div>
      </DocumentLayout>
    )
  }

  if (contentIsHtml) {
    return (
      <PublicLayout showMainContainer={false}>
        <RichContent mode='html' htmlVariant='isolated' content={rawContent} />
      </PublicLayout>
    )
  }

  return (
    <DocumentLayout title={title}>
      <RichContent
        mode='markdown'
        content={rawContent}
        className='prose-neutral dark:prose-invert max-w-none'
      />
    </DocumentLayout>
  )
}

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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { DEFAULT_SYSTEM_NAME } from '@/lib/constants'
import { cn } from '@/lib/utils'

import { GatewayAtlas } from '../gateway-atlas'

export function Hero(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const siteTitle = status?.system_name?.trim() || DEFAULT_SYSTEM_NAME
  const docsUrl =
    (typeof status?.docs_link === 'string' && status.docs_link.trim()) ||
    'https://docs.newapi.pro'
  const serverUrl =
    (typeof status?.server_address === 'string' &&
      status.server_address.trim()) ||
    window.location.origin

  const docsLabel = (
    <>
      <BookOpen size={16} aria-hidden='true' />
      {t('Docs')}
    </>
  )

  return (
    <section
      aria-labelledby='gateway-heading'
      className='gateway-hero gateway-container'
    >
      <div className='gateway-intro'>
        <div className='gateway-signature'>
          <span aria-hidden='true' className='gateway-signature-mark' />
          <span>{status?.home_badge?.trim() || siteTitle}</span>
        </div>
        <h1 id='gateway-heading'>
          <span>{status?.home_title_line1?.trim() || t('Many models.')}</span>{' '}
          <span>{status?.home_title_line2?.trim() || t('One gateway.')}</span>
        </h1>
        <p>
          {t(
            'Connect your apps to AI models through a single API. Manage keys, route requests and track spending in one console.'
          )}
        </p>
        <div className='gateway-actions'>
          <Link
            className={cn(buttonVariants({ size: 'lg' }), 'gateway-primary')}
            to={props.isAuthenticated ? '/dashboard' : '/sign-up'}
          >
            {props.isAuthenticated ? t('Go to Dashboard') : t('Get Started')}
            <ArrowRight aria-hidden='true' />
          </Link>
          {!props.isAuthenticated && (
            <Link
              className={buttonVariants({ variant: 'outline', size: 'lg' })}
              to='/pricing'
            >
              {t('View Pricing')}
            </Link>
          )}
        </div>
        {docsUrl.startsWith('http') ? (
          <a
            className='gateway-docs-link'
            href={docsUrl}
            target='_blank'
            rel='noopener noreferrer'
          >
            {docsLabel}
          </a>
        ) : (
          <Link className='gateway-docs-link' to={docsUrl}>
            {docsLabel}
          </Link>
        )}
      </div>
      <GatewayAtlas baseUrl={`${serverUrl.replace(/\/+$/, '')}/v1`} />
    </section>
  )
}

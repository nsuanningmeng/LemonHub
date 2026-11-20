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
import { ArrowUpRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { DEFAULT_SYSTEM_NAME } from '@/lib/constants'

import { GatewayAtlas } from '../gateway-atlas'

export function Hero(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const siteTitle = status?.system_name?.trim()
  const firstLine = status?.home_title_line1?.trim()
  const secondLine = status?.home_title_line2?.trim()
  const badge =
    status?.home_badge?.trim() ||
    (siteTitle !== DEFAULT_SYSTEM_NAME ? siteTitle : undefined)

  return (
    <section
      aria-labelledby='home-title'
      className='gateway-hero'
      data-custom-heading={firstLine || secondLine ? true : undefined}
    >
      <GatewayAtlas />
      <div id='gateway-heading' className='gateway-hero-content'>
        {badge && <div className='gateway-signature'>{badge}</div>}
        <h1 id='home-title'>
          <span>{firstLine || t('The developer’s')}</span>{' '}
          <span>{secondLine || t('AI gateway')}</span>
        </h1>
        <p>
          {t(
            'Use one API address for your models and see usage and cost for each request.'
          )}
        </p>
        <div className='gateway-actions'>
          <Link
            className='gateway-primary'
            to={props.isAuthenticated ? '/dashboard' : '/sign-up'}
          >
            {props.isAuthenticated ? t('Go to Dashboard') : t('Get Started')}{' '}
            <ArrowUpRight aria-hidden='true' />
          </Link>
          <Link className='gateway-secondary' to='/pricing'>
            {t('Browse models')}
          </Link>
        </div>
      </div>
    </section>
  )
}

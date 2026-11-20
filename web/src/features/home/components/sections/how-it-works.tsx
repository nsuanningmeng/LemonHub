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
import { ArrowUpRight, BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'

import { HeroTerminalDemo } from '../hero-terminal-demo'
import { ModelIntegrations } from './model-integrations'

export function HowItWorks() {
  const { t } = useTranslation()
  const { status } = useStatus()
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
      <ArrowUpRight size={14} aria-hidden='true' />
    </>
  )
  return (
    <section
      className='gateway-workbench gateway-container'
      aria-labelledby='workflow-title'
    >
      <div className='gateway-lab'>
        <header className='gateway-lab-heading'>
          <h2 id='workflow-title'>{t('Developer quickstart')}</h2>
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
        </header>
        <HeroTerminalDemo serverUrl={serverUrl.replace(/\/+$/, '')} />
      </div>
      <ModelIntegrations />
    </section>
  )
}

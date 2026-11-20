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
import Anthropic from '@lobehub/icons/es/Anthropic/components/Mono'
import DeepSeek from '@lobehub/icons/es/DeepSeek/components/Mono'
import Gemini from '@lobehub/icons/es/Gemini/components/Mono'
import OpenAI from '@lobehub/icons/es/OpenAI/components/Mono'
import { Link } from '@tanstack/react-router'
import { ArrowRight, ArrowUpRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const INTEGRATIONS = [
  {
    name: 'OpenAI',
    vendor: 'OpenAI',
    protocol: 'Chat / Responses',
    icon: OpenAI,
  },
  {
    name: 'Anthropic',
    vendor: 'Anthropic',
    protocol: 'Messages',
    icon: Anthropic,
  },
  {
    name: 'Gemini',
    vendor: 'Google',
    protocol: 'generateContent',
    icon: Gemini,
  },
  { name: 'DeepSeek', vendor: 'DeepSeek', protocol: 'Chat', icon: DeepSeek },
]

export function ModelIntegrations() {
  const { t } = useTranslation()
  return (
    <aside className='gateway-directory' aria-labelledby='integrations-title'>
      <h2 id='integrations-title'>{t('Model directory')}</h2>
      <ul>
        {INTEGRATIONS.map(({ name, vendor, protocol, icon: Icon }) => (
          <li key={name}>
            <Link
              to='/pricing'
              search={{ vendor }}
              aria-label={t('Browse {{provider}} models', { provider: name })}
            >
              <span className='gateway-provider-icon'>
                <Icon size={23} aria-hidden='true' />
              </span>
              <span>
                <strong>{name}</strong>
                <small>{protocol}</small>
              </span>
              <ArrowRight size={16} aria-hidden='true' />
            </Link>
          </li>
        ))}
      </ul>
      <div className='gateway-apps'>
        <h3>{t('Supported Applications')}</h3>
        <a
          href='https://cherry-ai.com'
          target='_blank'
          rel='noopener noreferrer'
        >
          Cherry Studio <ArrowUpRight size={14} aria-hidden='true' />
        </a>
        <a href='https://ccswitch.io' target='_blank' rel='noopener noreferrer'>
          CC Switch <ArrowUpRight size={14} aria-hidden='true' />
        </a>
      </div>
    </aside>
  )
}

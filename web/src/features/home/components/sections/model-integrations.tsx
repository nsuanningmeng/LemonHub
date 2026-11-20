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
import { useTranslation } from 'react-i18next'

const INTEGRATIONS = [
  { name: 'OpenAI', icon: OpenAI },
  { name: 'Anthropic', icon: Anthropic },
  { name: 'Gemini', icon: Gemini },
  { name: 'DeepSeek', icon: DeepSeek },
]

export function ModelIntegrations() {
  const { t } = useTranslation()
  return (
    <section
      className='gateway-integrations'
      aria-labelledby='integrations-title'
    >
      <div className='gateway-container gateway-integrations-row'>
        <h2 id='integrations-title'>{t('Model integrations')}</h2>
        <ul>
          {INTEGRATIONS.map(({ name, icon: Icon }) => (
            <li key={name}>
              <Icon size={26} aria-hidden='true' />
              <span>{name}</span>
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}

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
import { useTranslation } from 'react-i18next'

import { HeroTerminalDemo } from '../hero-terminal-demo'

export function HowItWorks() {
  const { t } = useTranslation()
  const steps = [
    [
      t('Configure'),
      t('Add your API keys, set up channels and configure access permissions'),
    ],
    [
      t('Connect'),
      t(
        'Connect through OpenAI, Claude, Gemini, and other compatible API routes'
      ),
    ],
    [
      t('Monitor'),
      t('Track usage, costs and performance with real-time analytics'),
    ],
  ]
  return (
    <section
      className='gateway-container gateway-workflow'
      aria-labelledby='workflow-title'
    >
      <div>
        <h2 id='workflow-title' className='gateway-section-title'>
          {t('Three steps to get started')}
        </h2>
        <ol className='gateway-steps'>
          {steps.map(([title, description], index) => (
            <li key={title}>
              <span aria-hidden='true'>
                {String(index + 1).padStart(2, '0')}
              </span>
              <div>
                <h3>{title}</h3>
                <p>{description}</p>
              </div>
            </li>
          ))}
        </ol>
      </div>
      <HeroTerminalDemo />
    </section>
  )
}

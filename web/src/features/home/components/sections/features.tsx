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
import { Gauge, Network, ReceiptText, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function Features() {
  const { t } = useTranslation()
  const capabilities = [
    {
      icon: Network,
      title: t('Load Balancing'),
      description: t(
        'Support for high concurrency with automatic load balancing'
      ),
    },
    {
      icon: ReceiptText,
      title: t('Transparent Billing'),
      description: t('Pay-as-you-go with real-time usage monitoring'),
    },
    {
      icon: ShieldCheck,
      title: t('Team Collaboration'),
      description: t(
        'Multi-user management with flexible permission allocation'
      ),
    },
    {
      icon: Gauge,
      title: t('Cost Tracking'),
      description: t(
        'Track usage, costs and performance with real-time analytics'
      ),
    },
  ]
  return (
    <section
      className='gateway-capabilities'
      aria-labelledby='capabilities-title'
    >
      <div className='gateway-container gateway-capabilities-inner'>
        <div>
          <h2 id='capabilities-title' className='gateway-section-title'>
            {t('Capabilities')}
          </h2>
        </div>
        <ul>
          {capabilities.map(({ icon: Icon, title, description }) => (
            <li key={title}>
              <Icon size={20} aria-hidden='true' />
              <div>
                <h3>{title}</h3>
                <p>{description}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}

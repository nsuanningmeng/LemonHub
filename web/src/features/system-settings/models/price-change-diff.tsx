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

import {
  PRICE_REVIEW_FIELDS,
  type ModelPriceChange,
  type UnreadablePriceChange,
} from './price-change-review'

export function PriceChangeDiff(props: {
  changes: ModelPriceChange[]
  unreadable: UnreadablePriceChange[]
}) {
  const { t } = useTranslation()
  return (
    <div className='max-h-[50dvh] space-y-4 overflow-auto'>
      {props.changes.map((change) => (
        <section key={change.name} className='space-y-2'>
          <h3 className='font-medium break-all'>{change.name}</h3>
          <table className='w-full table-fixed text-left text-sm'>
            <thead>
              <tr>
                <th>{t('Field')}</th>
                <th>{t('Current price')}</th>
                <th>{t('New price')}</th>
              </tr>
            </thead>
            <tbody>
              {PRICE_REVIEW_FIELDS.map(([field, label]) => {
                const before = change.saved[field]
                const after = change.draft[field]
                if (before === after) return null
                return (
                  <tr key={field}>
                    <th className='pr-2 align-top font-normal'>{t(label)}</th>
                    <td className='pr-2 align-top break-all whitespace-pre-wrap'>
                      {before ?? t('Not set')}
                    </td>
                    <td className='align-top break-all whitespace-pre-wrap'>
                      {after ?? t('Not set')}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </section>
      ))}
      {props.unreadable.map((change) => (
        <section key={change.field} className='space-y-2'>
          <h3 className='font-medium'>{t(change.label)}</h3>
          <p className='text-sm'>
            {t(
              'The saved configuration is invalid. Review the full replacement before saving.'
            )}
          </p>
          <div className='grid grid-cols-2 gap-3 text-sm'>
            <div>
              <h4>{t('Current price')}</h4>
              <pre className='break-all whitespace-pre-wrap'>
                {change.saved}
              </pre>
            </div>
            <div>
              <h4>{t('New price')}</h4>
              <pre className='break-all whitespace-pre-wrap'>
                {change.draft}
              </pre>
            </div>
          </div>
        </section>
      ))}
    </div>
  )
}

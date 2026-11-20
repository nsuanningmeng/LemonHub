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
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { SETUP_STEPS } from '../steps'

export function SetupProgress(props: { currentStep: number }) {
  const { t } = useTranslation()

  return (
    <ol
      aria-label={t('System setup wizard')}
      className='grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4'
    >
      {SETUP_STEPS.map((step, index) => {
        const active = props.currentStep === index
        const completed = props.currentStep > index
        return (
          <li
            key={step.titleKey}
            aria-current={active ? 'step' : undefined}
            className={cn(
              'border-border text-muted-foreground border-t-2 py-2',
              completed && 'border-primary/40',
              active && 'border-primary text-foreground'
            )}
          >
            <div className='flex items-baseline gap-2 text-sm font-semibold'>
              <span
                className='font-mono text-xs tabular-nums'
                aria-hidden='true'
              >
                {completed ? (
                  <Check className='inline size-3' />
                ) : (
                  String(index + 1).padStart(2, '0')
                )}
              </span>
              {t(step.titleKey)}
            </div>
            <p className='text-muted-foreground mt-1 hidden text-xs leading-relaxed sm:block'>
              {t(step.descriptionKey)}
            </p>
          </li>
        )
      })}
    </ol>
  )
}

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
      className='mt-5 grid grid-cols-4 gap-1 md:mt-8 md:grid-cols-1 md:gap-0'
    >
      {SETUP_STEPS.map((step, index) => {
        const active = props.currentStep === index
        const complete = props.currentStep > index
        return (
          <li
            key={step.titleKey}
            aria-current={active ? 'step' : undefined}
            className={cn(
              'border-border text-muted-foreground flex min-w-0 flex-col items-start gap-2 border-t-2 pt-2 md:flex-row md:items-center md:gap-3 md:border-t-0 md:py-2.5',
              complete && 'border-primary',
              active && 'border-foreground text-foreground font-semibold'
            )}
          >
            <span
              aria-hidden='true'
              className={cn(
                'grid size-6 shrink-0 place-items-center rounded-full font-mono text-xs tabular-nums',
                active
                  ? 'bg-brand-citrus text-brand-ink'
                  : 'bg-muted text-muted-foreground'
              )}
            >
              {complete ? <Check className='size-3.5' /> : index + 1}
            </span>
            <span className='text-xs leading-snug [overflow-wrap:anywhere] sm:text-sm'>
              {t(step.titleKey)}
            </span>
          </li>
        )
      })}
    </ol>
  )
}

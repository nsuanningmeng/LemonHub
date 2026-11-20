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
import { useNavigate, useRouter } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type ErrorPageProps = {
  code?: number
  title: string
  description: string
  className?: string
  minimal?: boolean
  action?: ReactNode
}

export function ErrorPage(props: ErrorPageProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { history } = useRouter()
  return (
    <section
      aria-label={props.title}
      className={cn(
        'bg-background text-foreground grid min-h-svh w-full place-items-center px-5 py-8',
        props.className
      )}
    >
      <div className='mx-auto flex w-full max-w-xl flex-col items-center justify-center gap-2 text-center'>
        {props.code && (
          <p className='text-[7rem] leading-tight font-bold tabular-nums'>
            <span className='sr-only'>HTTP </span>
            {props.code}
          </p>
        )}
        <h1 className='font-medium text-balance'>{props.title}</h1>
        <p className='text-muted-foreground max-w-prose leading-relaxed'>
          {props.description}
        </p>
        {(!props.minimal || props.action) && (
          <div className='mt-6 flex flex-wrap items-center justify-center gap-4'>
            {props.action}
            {!props.minimal && (
              <>
                <Button
                  variant={props.action ? 'outline' : 'default'}
                  onClick={() => navigate({ to: '/' })}
                >
                  {t('Back to Home')}
                </Button>
                {history.canGoBack() && (
                  <Button variant='outline' onClick={() => history.go(-1)}>
                    {t('Go Back')}
                  </Button>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </section>
  )
}

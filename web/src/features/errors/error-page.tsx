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
        'bg-background text-foreground grid min-h-svh content-center px-5 py-12 sm:px-10',
        props.className
      )}
    >
      <div className='border-foreground mx-auto w-full max-w-xl border-t-2 pt-5 md:pt-8'>
        {props.code && (
          <p className='text-muted-foreground mb-3 font-mono text-sm tabular-nums'>
            HTTP {props.code}
          </p>
        )}
        <h1 className='text-3xl leading-tight font-semibold tracking-tight text-balance md:text-5xl'>
          {props.title}
        </h1>
        <p className='text-muted-foreground mt-4 max-w-prose text-base leading-relaxed'>
          {props.description}
        </p>
        {(!props.minimal || props.action) && (
          <div className='mt-7 flex flex-wrap items-center gap-2'>
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
                  <Button variant='ghost' onClick={() => history.go(-1)}>
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

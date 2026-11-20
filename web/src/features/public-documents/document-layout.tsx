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
import type { ReactNode } from 'react'

import { PublicLayout } from '@/components/layout'
import { Skeleton } from '@/components/ui/skeleton'

export function DocumentLayout(props: {
  children: ReactNode
  title?: string
  footer?: ReactNode
}) {
  return (
    <PublicLayout showMainContainer={false}>
      <div className='flex min-h-svh flex-col'>
        <main className='mx-auto w-full max-w-[76rem] flex-1 px-5 pt-20 sm:px-8'>
          {props.title ? (
            <article className='mx-auto grid max-w-[72rem] gap-6 py-7 sm:gap-8 sm:py-14 md:grid-cols-[minmax(12rem,0.85fr)_minmax(0,1.6fr)] md:gap-14 md:py-20'>
              <h1 className='text-4xl leading-tight font-semibold tracking-tight text-balance [overflow-wrap:anywhere] md:text-5xl'>
                {props.title}
              </h1>
              <div className='border-foreground min-w-0 border-t-2 pt-5 leading-relaxed sm:pt-7'>
                {props.children}
              </div>
            </article>
          ) : (
            <article className='mx-auto w-full max-w-prose py-8 md:py-16'>
              {props.children}
            </article>
          )}
        </main>
        {props.footer}
      </div>
    </PublicLayout>
  )
}

export function DocumentLoading() {
  return (
    <DocumentLayout>
      <div aria-busy='true' className='space-y-4'>
        <Skeleton className='mb-10 h-10 w-2/3 max-w-xs' />
        <Skeleton className='h-4 w-full' />
        <Skeleton className='h-4 w-11/12' />
        <Skeleton className='h-4 w-4/5' />
      </div>
    </DocumentLayout>
  )
}

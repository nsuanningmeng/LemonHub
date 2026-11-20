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

export function DocumentLayout(props: { children: ReactNode }) {
  return (
    <PublicLayout>
      <div className='mx-auto w-full max-w-3xl py-8 md:py-16'>
        {props.children}
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

export function DocumentTitle(props: { children: ReactNode }) {
  return (
    <h1 className='mb-8 border-b pb-6 text-3xl font-semibold tracking-tight text-balance md:mb-10 md:text-5xl'>
      {props.children}
    </h1>
  )
}

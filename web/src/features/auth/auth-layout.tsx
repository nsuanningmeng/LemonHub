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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='relative grid min-h-svh max-w-none min-w-0'>
      <Link
        to='/'
        className='absolute top-4 right-4 left-4 z-10 flex min-w-0 items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:right-8 sm:left-8'
      >
        <div className='relative h-8 w-8 shrink-0'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-full' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-8 w-8 rounded-full object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1
            className='min-w-0 truncate text-xl font-medium'
            title={systemName}
          >
            {systemName}
          </h1>
        )}
      </Link>
      <div className='container flex min-w-0 items-center px-4 pt-16 sm:px-8 sm:pt-24'>
        <div className='mx-auto flex w-full max-w-[480px] min-w-0 flex-col justify-center space-y-2 py-8 sm:p-8'>
          {children}
        </div>
      </div>
    </div>
  )
}

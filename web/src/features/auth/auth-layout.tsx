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

import { LanguageSwitcher } from '@/components/language-switcher'
import { NavLinkItem } from '@/components/layout/components/nav-link-item'
import { ThemeSwitch } from '@/components/theme-switch'
import { Skeleton } from '@/components/ui/skeleton'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'

type AuthLayoutProps = {
  children: React.ReactNode
  title?: string
  description?: string
  secondaryAction?: React.ReactNode
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const { status } = useStatus()
  const docsUrl = status?.docs_link || '/docs'
  const supportLinks = useTopNavLinks().filter(
    (link) => link.href === docsUrl || link.href === '/contact'
  )

  return (
    <div className='bg-background text-foreground flex min-h-svh min-w-0 flex-col'>
      <header className='border-border bg-card border-b'>
        <div className='mx-auto flex min-h-16 max-w-[76rem] items-center justify-between gap-4 px-5 sm:px-8'>
          <Link
            to='/'
            aria-label={t('Go to home')}
            title={systemName}
            className='focus-visible:ring-ring flex max-w-full min-w-0 items-center gap-2 rounded-sm focus-visible:ring-2'
          >
            {loading ? (
              <Skeleton className='size-7 shrink-0' />
            ) : (
              <img
                src={logo}
                alt=''
                className='size-7 shrink-0 object-contain'
              />
            )}
            {loading ? (
              <Skeleton className='h-5 w-24' />
            ) : (
              <span className='min-w-0 truncate font-semibold tracking-tight md:[overflow-wrap:anywhere]'>
                {systemName}
              </span>
            )}
          </Link>
          <div className='flex shrink-0 items-center gap-1'>
            <LanguageSwitcher />
            <ThemeSwitch />
          </div>
        </div>
      </header>
      <main className='mx-auto w-full max-w-[70rem] flex-1 px-5 py-10 sm:px-8 sm:py-16 lg:py-24'>
        <div className='grid min-w-0 items-start gap-7 md:grid-cols-[minmax(0,0.93fr)_minmax(0,1fr)] md:gap-16 lg:gap-24'>
          <div className='min-w-0'>
            {props.title && (
              <h1 className='max-w-2xl text-4xl leading-[1.08] font-semibold tracking-tight text-balance [overflow-wrap:anywhere] sm:text-5xl'>
                {props.title}
              </h1>
            )}
            {props.description && (
              <p className='text-muted-foreground mt-4 max-w-sm text-base leading-relaxed text-pretty'>
                {props.description}
              </p>
            )}
          </div>
          <div className='border-foreground min-w-0 border-t-2 pt-6 md:pt-7 [&_button[type=submit]]:min-h-11 [&_input:not([type=checkbox])]:min-h-11'>
            {props.children}
            {props.secondaryAction && (
              <div className='text-muted-foreground mt-6 border-t pt-5 text-sm leading-relaxed'>
                {props.secondaryAction}
              </div>
            )}
          </div>
        </div>
      </main>
      {supportLinks.length > 0 && (
        <footer className='border-t'>
          <nav
            aria-label={t('Resources')}
            className='mx-auto flex min-h-14 max-w-[76rem] flex-wrap items-center gap-x-6 gap-y-2 px-5 py-3 sm:px-8'
          >
            {supportLinks.map((link) => (
              <NavLinkItem
                key={link.href}
                link={link}
                className='text-muted-foreground hover:text-foreground text-sm underline-offset-4 hover:underline'
              />
            ))}
          </nav>
        </footer>
      )}
    </div>
  )
}

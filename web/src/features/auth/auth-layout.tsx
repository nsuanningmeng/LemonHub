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
import { ArrowUpRight } from 'lucide-react'
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
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  const { status } = useStatus()
  const docsUrl = status?.docs_link || '/docs'
  const supportLinks = useTopNavLinks().filter(
    (link) =>
      link.href === docsUrl ||
      link.href === '/contact' ||
      (link.href === '/pricing' && !link.requiresAuth && !link.disabled)
  )

  return (
    <div className='bg-card sm:bg-muted/50 flex min-h-svh min-w-0 flex-col items-center sm:justify-center sm:p-8'>
      <div className='bg-card relative grid w-full max-w-[62rem] min-w-0 sm:overflow-hidden sm:rounded-lg sm:border md:grid-cols-[minmax(0,0.82fr)_minmax(0,1.18fr)]'>
        <header className='flex min-w-0 flex-col bg-(--brand-ink) px-5 py-5 pe-28 text-white sm:p-8 sm:pe-28 md:min-h-112 md:p-10 md:pe-10'>
          <Link
            to='/'
            aria-label={t('Go to home')}
            title={systemName}
            className='inline-flex max-w-full min-w-0 items-center gap-3 self-start rounded-sm focus-visible:ring-2 focus-visible:ring-(--brand-citrus) md:mt-auto md:block'
          >
            {loading ? (
              <Skeleton className='size-8 shrink-0 rounded-md' />
            ) : (
              <img
                src={logo}
                alt=''
                className='size-8 shrink-0 rounded-md bg-white object-contain md:mb-5 md:size-12'
              />
            )}
            {loading ? (
              <Skeleton className='h-6 w-24' />
            ) : (
              <span className='block min-w-0 truncate text-xl font-semibold tracking-tight md:text-4xl md:leading-tight md:[overflow-wrap:anywhere] md:whitespace-normal'>
                {systemName}
              </span>
            )}
          </Link>
          <p className='mt-3 hidden font-mono text-sm break-all text-white/75 md:block'>
            {window.location.host}
          </p>
          {supportLinks.length > 0 && (
            <nav
              aria-label={t('Resources')}
              className='mt-auto hidden gap-4 border-t border-white/25 pt-4 md:flex md:flex-wrap'
            >
              {supportLinks.map((link) => (
                <span
                  key={link.href}
                  className='inline-flex items-center gap-1'
                >
                  <NavLinkItem
                    link={
                      link.href === '/pricing'
                        ? { ...link, title: 'Explore models' }
                        : link
                    }
                    className='text-sm text-white/85 hover:text-white hover:underline'
                  />
                  <ArrowUpRight
                    aria-hidden='true'
                    className='size-3.5 text-(--brand-citrus)'
                  />
                </span>
              ))}
            </nav>
          )}
        </header>
        <main className='flex min-w-0 flex-col px-5 pt-6 pb-7 sm:px-9 sm:pt-8 sm:pb-10 md:px-12 md:pt-0'>
          <div className='md:text-foreground absolute end-4 top-4 flex justify-end gap-1 text-white sm:end-8 sm:top-7 md:static md:py-3'>
            <LanguageSwitcher />
            <ThemeSwitch />
          </div>
          <section
            aria-label={t('Authentication')}
            className='my-auto w-full min-w-0 pt-3 pb-4 md:py-5 [&_button[type=submit]]:min-h-10 [&_input:not([type=checkbox])]:min-h-10'
          >
            {props.children}
          </section>
          {supportLinks.length > 0 && (
            <nav
              aria-label={t('Resources')}
              className='mt-3 flex flex-wrap gap-x-5 gap-y-2 border-t pt-3 md:hidden'
            >
              {supportLinks.map((link) => (
                <NavLinkItem
                  key={link.href}
                  link={
                    link.href === '/pricing'
                      ? { ...link, title: 'Explore models' }
                      : link
                  }
                  className='text-muted-foreground text-sm underline-offset-4 hover:underline'
                />
              ))}
            </nav>
          )}
        </main>
      </div>
    </div>
  )
}

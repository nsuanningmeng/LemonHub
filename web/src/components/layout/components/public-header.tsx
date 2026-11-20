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
import { Link, useNavigate } from '@tanstack/react-router'
import { ArrowUpRight, Menu, X } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { useNotifications } from '@/hooks/use-notifications'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { defaultTopNavLinks } from '../config/top-nav.config'
import type { TopNavLink } from '../types'
import { HeaderLogo } from './header-logo'
import { NavLinkList } from './nav-link-item'

const AUTH_PROMPT_SECONDS = 5

type AuthPromptTarget = {
  title: string
  href: string
}

export interface PublicHeaderProps {
  navLinks?: TopNavLink[]
  mobileLinks?: TopNavLink[]
  navContent?: React.ReactNode
  showThemeSwitch?: boolean
  showLanguageSwitcher?: boolean
  logo?: React.ReactNode
  siteName?: string
  homeUrl?: string
  leftContent?: React.ReactNode
  rightContent?: React.ReactNode
  showNavigation?: boolean
  showAuthButtons?: boolean
  showNotifications?: boolean
  className?: string
}

export function PublicHeader(props: PublicHeaderProps) {
  const {
    navLinks = defaultTopNavLinks,
    showThemeSwitch = true,
    showLanguageSwitcher = true,
    logo: customLogo,
    siteName: customSiteName,
    homeUrl = '/',
    showAuthButtons = true,
    showNotifications = true,
  } = props

  const { t } = useTranslation()
  const navigate = useNavigate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [authPromptTarget, setAuthPromptTarget] =
    useState<AuthPromptTarget | null>(null)
  const [authPromptSecondsLeft, setAuthPromptSecondsLeft] =
    useState(AUTH_PROMPT_SECONDS)
  const user = useAuthStore((state) => state.auth.user)
  const {
    systemName,
    logo: systemLogo,
    loading,
    logoLoaded,
  } = useSystemConfig()
  const dynamicLinks = useTopNavLinks()
  const notifications = useNotifications()
  const isAuthenticated = !!user
  const displaySiteName = customSiteName || systemName
  const links = dynamicLinks.length > 0 ? dynamicLinks : navLinks

  useEffect(() => {
    if (!authPromptTarget) return

    const intervalId = window.setInterval(() => {
      setAuthPromptSecondsLeft((seconds) => Math.max(seconds - 1, 0))
    }, 1000)

    const timeoutId = window.setTimeout(() => {
      const redirect = authPromptTarget.href
      setAuthPromptTarget(null)
      navigate({ to: '/sign-in', search: { redirect } })
    }, AUTH_PROMPT_SECONDS * 1000)

    return () => {
      window.clearInterval(intervalId)
      window.clearTimeout(timeoutId)
    }
  }, [authPromptTarget, navigate])

  const closeAuthPrompt = useCallback(() => {
    setAuthPromptTarget(null)
    setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
  }, [])

  const navigateToSignIn = useCallback(() => {
    const redirect = authPromptTarget?.href || '/'
    setAuthPromptTarget(null)
    navigate({ to: '/sign-in', search: { redirect } })
  }, [authPromptTarget?.href, navigate])

  const handleNavLinkClick = useCallback(
    (
      event: React.MouseEvent<HTMLAnchorElement>,
      link: TopNavLink,
      closeMobile = false
    ) => {
      if (link.disabled) {
        event.preventDefault()
        return
      }

      if (link.requiresAuth) {
        event.preventDefault()
        if (closeMobile) {
          setMobileOpen(false)
        }
        setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
        setAuthPromptTarget({
          title: t(link.title),
          href: link.href,
        })
        return
      }

      if (closeMobile) {
        setMobileOpen(false)
      }
    },
    [t]
  )

  return (
    <>
      <header
        className={cn(
          'bg-background fixed inset-x-0 top-0 z-40 border-b',
          props.className
        )}
      >
        <div className='mx-auto flex h-16 max-w-[76rem] items-center gap-3 px-5 md:px-8'>
          <Link
            to={homeUrl}
            className='text-foreground focus-visible:ring-ring inline-flex max-w-56 min-w-0 items-center gap-2.5 rounded-sm focus-visible:ring-2'
          >
            <span className='flex size-7 shrink-0 items-center justify-center'>
              {loading && <Skeleton className='size-full rounded-md' />}
              {!loading &&
                (customLogo ?? (
                  <HeaderLogo
                    src={systemLogo}
                    alt=''
                    loading={loading}
                    logoLoaded={logoLoaded}
                    className='size-full rounded-none object-contain'
                  />
                ))}
            </span>
            <span
              className='truncate text-lg font-semibold tracking-tight'
              title={displaySiteName}
            >
              {loading ? <Skeleton className='h-5 w-20' /> : displaySiteName}
            </span>
          </Link>
          {props.leftContent}

          {props.showNavigation !== false && (
            <nav className='ms-2 hidden min-w-0 flex-1 items-center gap-0.5 overflow-x-auto xl:flex'>
              {props.navContent ?? (
                <NavLinkList
                  links={links}
                  onNavigate={handleNavLinkClick}
                  className='hover:bg-muted aria-[current=page]:text-foreground aria-[current=page]:decoration-primary shrink-0 rounded-sm px-2 py-2 text-sm font-medium whitespace-nowrap underline-offset-[5px] aria-[current=page]:underline'
                />
              )}
            </nav>
          )}

          <div className='ms-auto flex shrink-0 items-center gap-0.5 sm:gap-1'>
            {showLanguageSwitcher && <LanguageSwitcher />}
            {showThemeSwitch && <ThemeSwitch />}
            {showNotifications && (
              <NotificationPopover
                open={notifications.popoverOpen}
                onOpenChange={notifications.setPopoverOpen}
                unreadCount={notifications.unreadCount}
                activeTab={notifications.activeTab}
                onTabChange={notifications.setActiveTab}
                notice={notifications.notice}
                announcements={notifications.announcements}
                loading={notifications.loading}
              />
            )}
            {showAuthButtons && (
              <div
                className={cn(
                  'ms-1 border-s ps-2',
                  !isAuthenticated && 'hidden sm:block'
                )}
              >
                {loading && <Skeleton className='h-8 w-18 rounded-md' />}
                {!loading &&
                  (isAuthenticated ? (
                    <ProfileDropdown />
                  ) : (
                    <Button size='sm' render={<Link to='/sign-in' />}>
                      {t('Sign in')}
                      <ArrowUpRight aria-hidden='true' />
                    </Button>
                  ))}
              </div>
            )}
            {props.rightContent}

            <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
              <SheetTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon'
                    className='size-10 xl:hidden'
                  />
                }
                aria-label={t('Toggle navigation menu')}
              >
                <Menu aria-hidden='true' />
              </SheetTrigger>
              <SheetContent
                side='right'
                showCloseButton={false}
                className='w-[min(85vw,21rem)] gap-0 overflow-y-auto'
              >
                <div className='flex min-h-16 items-center justify-between gap-4 border-b px-5'>
                  <SheetTitle className='min-w-0 truncate'>
                    {displaySiteName}
                  </SheetTitle>
                  <SheetClose
                    render={
                      <Button
                        variant='ghost'
                        size='icon'
                        aria-label={t('Close menu')}
                      />
                    }
                  >
                    <X aria-hidden='true' />
                  </SheetClose>
                </div>
                {props.showNavigation !== false && (
                  <nav className='flex flex-col px-4 py-3'>
                    <NavLinkList
                      links={props.mobileLinks ?? links}
                      onNavigate={(event, link) =>
                        handleNavLinkClick(event, link, true)
                      }
                      className='hover:bg-muted aria-[current=page]:bg-accent aria-[current=page]:text-accent-foreground flex min-h-11 items-center rounded-md px-3 py-2 text-base font-medium'
                    />
                  </nav>
                )}
                {showAuthButtons && (
                  <div className='mt-auto border-t p-5'>
                    <Button
                      className='h-11 w-full'
                      render={
                        <Link
                          to={isAuthenticated ? '/dashboard' : '/sign-in'}
                          onClick={() => setMobileOpen(false)}
                        />
                      }
                    >
                      {isAuthenticated ? t('Go to Dashboard') : t('Sign in')}
                      <ArrowUpRight aria-hidden='true' />
                    </Button>
                  </div>
                )}
              </SheetContent>
            </Sheet>
          </div>
        </div>
      </header>

      <Dialog
        open={!!authPromptTarget}
        onOpenChange={(open) => {
          if (!open) {
            closeAuthPrompt()
          }
        }}
        title={t('Sign in required')}
        description={t('Please sign in to view {{module}}.', {
          module: authPromptTarget?.title || '',
        })}
        contentClassName='sm:max-w-md'
        contentHeight='auto'
        footer={
          <>
            <Button variant='outline' onClick={closeAuthPrompt}>
              {t('Cancel')}
            </Button>
            <Button onClick={navigateToSignIn}>{t('Sign in now')}</Button>
          </>
        }
      >
        <div className='bg-muted/40 text-muted-foreground rounded-lg px-3 py-2 text-sm'>
          {t('Redirecting to sign in in {{seconds}} seconds.', {
            seconds: authPromptSecondsLeft,
          })}
        </div>
      </Dialog>
    </>
  )
}

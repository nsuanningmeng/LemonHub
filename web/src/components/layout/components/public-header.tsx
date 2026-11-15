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
import { HStack } from '@astryxdesign/core/HStack'
import { Text } from '@astryxdesign/core/Text'
import { VStack } from '@astryxdesign/core/VStack'
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { ArrowUpRight, ChevronDown, ChevronRight, Menu } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Sheet,
  SheetContent,
  SheetDescription,
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

const AUTH_PROMPT_SECONDS = 5

type AuthPromptTarget = { title: string; href: string }

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

function PublicNavLink(props: {
  link: TopNavLink
  pathname: string
  className?: string
  onClick: (event: React.MouseEvent<HTMLAnchorElement>) => void
}) {
  const { t } = useTranslation()
  const className = cn(
    'hover:text-tone-blue-strong hover:bg-tone-blue focus-visible:ring-ring inline-flex min-w-0 items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors focus-visible:ring-2 focus-visible:outline-none',
    props.pathname === props.link.href
      ? 'surface-blue font-semibold'
      : 'text-muted-foreground',
    props.link.disabled && 'pointer-events-none opacity-50',
    props.className
  )

  if (props.link.external) {
    return (
      <a
        href={props.link.href}
        target='_blank'
        rel='noopener noreferrer'
        aria-disabled={props.link.disabled}
        tabIndex={props.link.disabled ? -1 : undefined}
        onClick={props.onClick}
        className={className}
      >
        {t(props.link.title)}
        <ArrowUpRight className='size-3' aria-hidden='true' />
      </a>
    )
  }
  return (
    <Link
      to={props.link.href}
      disabled={props.link.disabled}
      aria-current={props.pathname === props.link.href ? 'page' : undefined}
      onClick={props.onClick}
      className={className}
    >
      {t(props.link.title)}
      <ChevronRight
        className='ms-auto size-4 opacity-50 lg:hidden'
        aria-hidden='true'
      />
    </Link>
  )
}

export function PublicHeader(props: PublicHeaderProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [authPromptTarget, setAuthPromptTarget] =
    useState<AuthPromptTarget | null>(null)
  const [authPromptSecondsLeft, setAuthPromptSecondsLeft] =
    useState(AUTH_PROMPT_SECONDS)
  const user = useAuthStore((state) => state.auth.user)
  const system = useSystemConfig()
  const dynamicLinks = useTopNavLinks()
  const notifications = useNotifications()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const links =
    dynamicLinks.length > 0
      ? dynamicLinks
      : props.navLinks || defaultTopNavLinks
  const mobileLinks = props.mobileLinks || links
  const isAuthenticated = !!user
  const showNavigation = props.showNavigation !== false
  const showAuthButtons = props.showAuthButtons !== false
  const showLanguageSwitcher = props.showLanguageSwitcher !== false
  const showThemeSwitch = props.showThemeSwitch !== false
  const showNotifications = props.showNotifications !== false

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

  const handleNavLinkClick = (
    event: React.MouseEvent<HTMLElement>,
    link: TopNavLink
  ) => {
    if (link.disabled) {
      event.preventDefault()
      return
    }
    setMobileOpen(false)
    if (link.requiresAuth) {
      event.preventDefault()
      setAuthPromptSecondsLeft(AUTH_PROMPT_SECONDS)
      setAuthPromptTarget({ title: t(link.title), href: link.href })
    }
  }

  let desktopAuthContent: React.ReactNode = null
  if (showAuthButtons) {
    if (system.loading) {
      desktopAuthContent = <Skeleton className='h-9 w-20 rounded-full' />
    } else if (isAuthenticated) {
      desktopAuthContent = <ProfileDropdown />
    } else {
      desktopAuthContent = (
        <Button
          role='link'
          nativeButton={false}
          size='sm'
          className='surface-brand hover:bg-primary/90 ml-2 h-8 rounded-lg px-4'
          render={<Link to='/sign-in' />}
        >
          {t('Sign in')}
          <ArrowUpRight className='size-3.5' aria-hidden='true' />
        </Button>
      )
    }
  }

  return (
    <>
      <header
        className={cn(
          'fixed inset-x-0 top-0 z-40 h-16 px-3 py-2 sm:px-5',
          props.className
        )}
      >
        <HStack
          gap={4}
          align='center'
          justify='between'
          className='bg-background/95 border-tone-blue-border mx-auto h-full max-w-7xl rounded-xl border px-3 shadow-sm backdrop-blur-md md:px-4'
        >
          <HStack gap={3} align='center' className='min-w-0'>
            <Link
              to={props.homeUrl || '/'}
              className='inline-flex max-w-64 min-w-0 items-center gap-2.5'
            >
              {props.logo || (
                <HeaderLogo
                  src={system.logo}
                  loading={system.loading}
                  logoLoaded={system.logoLoaded}
                  className='size-8 shrink-0 rounded-lg object-contain'
                />
              )}
              {system.loading ? (
                <Skeleton className='h-5 w-24' />
              ) : (
                <Text
                  type='label'
                  color='inherit'
                  className='min-w-0 truncate text-base font-semibold tracking-tight'
                >
                  {props.siteName || system.systemName}
                </Text>
              )}
            </Link>
            {props.leftContent}
          </HStack>
          <HStack gap={1} align='center' className='hidden lg:flex'>
            {showNavigation && (
              <nav
                aria-label={t('Main navigation')}
                className='flex items-center gap-0.5'
              >
                {props.navContent ||
                  links
                    .slice(0, 4)
                    .map((link) => (
                      <PublicNavLink
                        key={link.href}
                        link={link}
                        pathname={pathname}
                        onClick={(event) => handleNavLinkClick(event, link)}
                      />
                    ))}
                {!props.navContent && links.length > 4 && (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant='ghost'
                          className='text-muted-foreground text-sm font-normal'
                        />
                      }
                    >
                      {t('More')}{' '}
                      <ChevronDown className='size-3.5' aria-hidden='true' />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align='end'>
                      {links.slice(4).map((link) => (
                        <DropdownMenuItem
                          key={link.href}
                          disabled={link.disabled}
                          onClick={(event) => handleNavLinkClick(event, link)}
                          render={
                            link.external ? (
                              <a
                                href={link.href}
                                target='_blank'
                                rel='noopener noreferrer'
                              />
                            ) : (
                              <Link to={link.href} />
                            )
                          }
                        >
                          {t(link.title)}
                          {link.external && (
                            <ArrowUpRight
                              className='size-3.5'
                              aria-hidden='true'
                            />
                          )}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </nav>
            )}
            <HStack
              gap={1}
              align='center'
              className='border-tone-blue-border ml-2 border-l pl-3'
            >
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
              {props.rightContent}
              {desktopAuthContent}
            </HStack>
          </HStack>
          <HStack gap={2} align='center' className='lg:hidden'>
            {showThemeSwitch && <ThemeSwitch />}
            {showAuthButtons && isAuthenticated && <ProfileDropdown />}
            <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
              <SheetTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon'
                    className='surface-blue size-8 rounded-lg'
                    aria-label={t('Toggle navigation menu')}
                  />
                }
              >
                <Menu className='size-5' aria-hidden='true' />
              </SheetTrigger>
              <SheetContent className='w-full max-w-sm gap-3 p-4'>
                <HStack
                  gap={3}
                  vAlign='center'
                  className='surface-blue mt-6 rounded-2xl p-5'
                >
                  <HeaderLogo
                    src={system.logo}
                    loading={system.loading}
                    logoLoaded={system.logoLoaded}
                    className='size-10 shrink-0 rounded-xl object-contain'
                  />
                  <VStack gap={2} className='min-w-0'>
                    <SheetTitle className='truncate text-xl font-semibold'>
                      {props.siteName || system.systemName}
                    </SheetTitle>
                    <SheetDescription className='text-tone-blue-strong'>
                      {t('Your models. Your workspace.')}
                    </SheetDescription>
                  </VStack>
                </HStack>
                <VStack gap={2} className='min-h-0 flex-1 overflow-y-auto py-6'>
                  {showNavigation && (
                    <nav
                      aria-label={t('Mobile navigation')}
                      className='flex flex-col gap-2'
                    >
                      {mobileLinks.map((link) => (
                        <PublicNavLink
                          key={link.href}
                          link={link}
                          pathname={pathname}
                          className='border-border border-b py-4 text-base font-medium'
                          onClick={(event) => handleNavLinkClick(event, link)}
                        />
                      ))}
                    </nav>
                  )}
                  {props.navContent}
                </VStack>
                <HStack gap={2} align='center'>
                  {showLanguageSwitcher && <LanguageSwitcher />}
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
                  {props.rightContent}
                </HStack>
                {showAuthButtons && (
                  <Button
                    role='link'
                    nativeButton={false}
                    size='lg'
                    className='surface-brand rounded-xl'
                    render={
                      <Link
                        to={isAuthenticated ? '/dashboard' : '/sign-in'}
                        onClick={() => setMobileOpen(false)}
                      />
                    }
                  >
                    {isAuthenticated ? t('Open your workspace') : t('Sign in')}
                    <ArrowUpRight className='size-4' aria-hidden='true' />
                  </Button>
                )}
              </SheetContent>
            </Sheet>
          </HStack>
        </HStack>
      </header>
      <Dialog
        open={!!authPromptTarget}
        onOpenChange={(open) => {
          if (!open) closeAuthPrompt()
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
        <Text
          as='p'
          type='supporting'
          color='inherit'
          className='surface-blue rounded-lg px-3 py-2 text-sm'
        >
          {t('Redirecting to sign in in {{seconds}} seconds.', {
            seconds: authPromptSecondsLeft,
          })}
        </Text>
      </Dialog>
    </>
  )
}

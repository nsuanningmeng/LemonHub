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
import { ChevronDown, Grid2X2 } from 'lucide-react'
import type { HTMLAttributes, MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type { TopNavLink } from '../types'

type TopNavProps = HTMLAttributes<HTMLElement> & {
  links: TopNavLink[]
  label?: string
  onNavigate?: (event: MouseEvent<HTMLAnchorElement>, link: TopNavLink) => void
}

export function TopNav(props: TopNavProps) {
  const { t } = useTranslation()
  const { links, label: customLabel, onNavigate, ...wrapperProps } = props
  const label = customLabel ?? t('Platform')

  return (
    <div {...wrapperProps}>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger
          aria-label={label}
          render={
            <Button
              variant='ghost'
              className='h-9 gap-1.5 px-2 text-sm font-medium'
            />
          }
        >
          {!customLabel && <Grid2X2 aria-hidden='true' className='size-4' />}
          <span className={customLabel ? undefined : 'hidden md:inline'}>
            {label}
          </span>
          <ChevronDown
            aria-hidden='true'
            className={cn('size-3.5', !customLabel && 'hidden md:block')}
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent side='bottom' align='end' className='min-w-44'>
          {links.map((link) => (
            <DropdownMenuItem
              key={link.href + link.title}
              disabled={link.disabled}
              render={
                link.external ? (
                  <a
                    href={link.disabled ? undefined : link.href}
                    target='_blank'
                    rel='noopener noreferrer'
                    onClick={(event) => onNavigate?.(event, link)}
                  />
                ) : (
                  <Link
                    to={link.href}
                    disabled={link.disabled}
                    activeOptions={{ exact: true }}
                    onClick={(event) => onNavigate?.(event, link)}
                  />
                )
              }
            >
              {t(link.title)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

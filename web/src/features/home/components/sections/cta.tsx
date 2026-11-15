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
import { Heading } from '@astryxdesign/core/Heading'
import { HStack } from '@astryxdesign/core/HStack'
import { Text } from '@astryxdesign/core/Text'
import { VStack } from '@astryxdesign/core/VStack'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, Orbit } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()
  return (
    <section
      className={cn(
        'mx-auto max-w-7xl px-6 pb-16 md:px-10 md:pb-24',
        props.className
      )}
      aria-labelledby='home-cta-title'
    >
      <HStack
        gap={8}
        wrap='wrap'
        align='center'
        justify='between'
        className='surface-brand rounded-3xl rounded-tr-4xl p-8 md:p-12'
      >
        <VStack gap={6} className='max-w-2xl'>
          <HStack gap={3} align='center'>
            <Orbit className='size-6' aria-hidden='true' />
            <Text
              type='supporting'
              color='inherit'
              className='tracking-widest uppercase'
            >
              {t('Your next chapter starts here')}
            </Text>
          </HStack>
          <Heading
            level={2}
            id='home-cta-title'
            type='display-2'
            color='inherit'
            className='text-3xl tracking-tight text-balance md:text-5xl'
          >
            {t('Good ideas deserve a clear runway.')}
          </Heading>
        </VStack>
        <Button
          role='link'
          nativeButton={false}
          size='lg'
          variant='ghost'
          className='surface-teal hover:bg-tone-teal hover:text-tone-teal-foreground h-12 rounded-2xl px-6 hover:opacity-90'
          render={
            <Link to={props.isAuthenticated ? '/dashboard' : '/sign-up'} />
          }
        >
          {props.isAuthenticated
            ? t('Open your workspace')
            : t('Create your workspace')}
          <ArrowUpRight className='size-4' aria-hidden='true' />
        </Button>
      </HStack>
    </section>
  )
}

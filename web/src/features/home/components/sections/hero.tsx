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
import { Grid } from '@astryxdesign/core/Grid'
import { Heading } from '@astryxdesign/core/Heading'
import { HStack } from '@astryxdesign/core/HStack'
import { Text } from '@astryxdesign/core/Text'
import { VStack } from '@astryxdesign/core/VStack'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, Braces, Layers3, Orbit, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { cn } from '@/lib/utils'

import { HeroTerminalDemo } from '../hero-terminal-demo'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const heroBadge =
    status?.home_badge?.trim() || t('An open canvas for your next idea')
  const heroTitleLine1 = status?.home_title_line1?.trim() || t('Big ideas.')
  const heroTitleLine2 =
    status?.home_title_line2?.trim() || t('One connection.')
  const docsUrl =
    typeof status?.docs_link === 'string' ? status.docs_link.trim() : ''

  return (
    <section
      aria-labelledby='home-title'
      className={cn(
        'mx-auto max-w-7xl px-6 pt-28 pb-16 md:px-10 md:pt-36 md:pb-20',
        props.className
      )}
    >
      <Grid columns={{ minWidth: 360, max: 2 }} gap={8} align='center'>
        <VStack gap={8} className='min-w-0 py-6 lg:pr-4'>
          <HStack gap={3} align='center'>
            <HStack
              align='center'
              justify='center'
              className='surface-violet size-10 shrink-0 rounded-2xl'
            >
              <Sparkles className='size-5' aria-hidden='true' />
            </HStack>
            <Text
              type='supporting'
              className='text-tone-blue-strong font-semibold tracking-wide'
            >
              {heroBadge}
            </Text>
          </HStack>
          <VStack gap={6}>
            <Heading
              level={1}
              id='home-title'
              type='display-1'
              className='text-5xl leading-tight tracking-tighter text-balance md:text-6xl xl:text-7xl'
            >
              {heroTitleLine1}
              <Text
                type='inherit'
                color='inherit'
                display='block'
                className='text-tone-blue-strong'
              >
                {heroTitleLine2}
              </Text>
            </Heading>
            <Text
              as='p'
              type='large'
              color='secondary'
              className='max-w-md leading-relaxed font-normal'
            >
              {t(
                'Bring your models, keys and usage into one thoughtful workspace. Connect once, then get back to what you are building.'
              )}
            </Text>
          </VStack>
          <HStack gap={3} wrap='wrap' align='center'>
            <Button
              role='link'
              nativeButton={false}
              size='lg'
              className='surface-brand h-12 rounded-2xl px-6 hover:opacity-90'
              render={
                <Link to={props.isAuthenticated ? '/dashboard' : '/sign-up'} />
              }
            >
              {props.isAuthenticated
                ? t('Open your workspace')
                : t('Start building')}
              <ArrowUpRight className='size-4' aria-hidden='true' />
            </Button>
            <Button
              role='link'
              nativeButton={false}
              size='lg'
              variant='ghost'
              className='h-12 rounded-2xl px-5'
              render={<Link to='/pricing' />}
            >
              {t('Explore the models')}
              <ArrowUpRight className='size-4' aria-hidden='true' />
            </Button>
          </HStack>
          <HStack gap={4} align='center' className='text-muted-foreground'>
            <HStack gap={0} aria-hidden='true'>
              <HStack
                justify='center'
                align='center'
                className='surface-blue ring-background size-9 rounded-full ring-4'
              >
                <Braces className='size-4' />
              </HStack>
              <HStack
                justify='center'
                align='center'
                className='surface-teal ring-background -ml-2 size-9 rounded-full ring-4'
              >
                <Orbit className='size-4' />
              </HStack>
              <HStack
                justify='center'
                align='center'
                className='surface-violet ring-background -ml-2 size-9 rounded-full ring-4'
              >
                <Layers3 className='size-4' />
              </HStack>
            </HStack>
            <VStack gap={1}>
              <Text type='supporting' color='inherit'>
                {t('Your stack. Your choice.')}
              </Text>
              <Text type='supporting' color='inherit' className='text-xs'>
                OpenAI · Anthropic · Gemini
              </Text>
            </VStack>
            {docsUrl && (
              <a
                href={docsUrl}
                target={docsUrl.startsWith('http') ? '_blank' : undefined}
                rel='noopener noreferrer'
                className='text-tone-blue-strong ml-auto text-sm underline underline-offset-4'
              >
                {t('Docs')}
              </a>
            )}
          </HStack>
        </VStack>
        <VStack
          gap={4}
          className='surface-blue min-w-0 rounded-3xl rounded-tr-4xl p-4 md:p-5'
        >
          <HStack
            gap={4}
            align='center'
            justify='between'
            className='surface-brand rounded-2xl rounded-tr-3xl px-5 py-5'
          >
            <VStack gap={1}>
              <Text
                type='supporting'
                color='inherit'
                className='text-xs tracking-widest uppercase'
              >
                {t('The connection studio')}
              </Text>
              <Text type='large' color='inherit' weight='semibold'>
                {t('Choose a protocol. Make it yours.')}
              </Text>
            </VStack>
            <Orbit
              className='size-10 shrink-0'
              strokeWidth={1.25}
              aria-hidden='true'
            />
          </HStack>
          <HeroTerminalDemo />
          <Grid columns={{ minWidth: 140, max: 2 }} gap={3}>
            <HStack
              gap={3}
              align='center'
              className='surface-teal rounded-2xl px-4 py-4'
            >
              <Braces className='size-5 shrink-0' aria-hidden='true' />
              <Text type='supporting' color='inherit' weight='medium'>
                {t('Familiar protocols')}
              </Text>
            </HStack>
            <HStack
              gap={3}
              align='center'
              className='surface-violet rounded-2xl px-4 py-4'
            >
              <Layers3 className='size-5 shrink-0' aria-hidden='true' />
              <Text type='supporting' color='inherit' weight='medium'>
                {t('Room for every model')}
              </Text>
            </HStack>
          </Grid>
        </VStack>
      </Grid>
    </section>
  )
}

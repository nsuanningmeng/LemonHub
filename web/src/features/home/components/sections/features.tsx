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
import {
  ArrowDown,
  ArrowUpRight,
  ChartNoAxesCombined,
  KeyRound,
  Route,
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

interface FeaturesProps {
  className?: string
}

export function Features(props: FeaturesProps) {
  const { t } = useTranslation()
  return (
    <section
      className={cn(
        'mx-auto max-w-7xl px-6 py-20 md:px-10 md:py-24',
        props.className
      )}
      aria-labelledby='home-features-title'
    >
      <VStack gap={10}>
        <HStack gap={8} wrap='wrap' justify='between' align='end'>
          <VStack gap={4} className='max-w-2xl'>
            <Text
              type='supporting'
              className='text-tone-blue-strong font-mono tracking-widest uppercase'
            >
              {t('Built around your workflow')}
            </Text>
            <Heading
              level={2}
              id='home-features-title'
              type='display-2'
              className='text-3xl leading-tight tracking-tight text-balance md:text-5xl'
            >
              {t('Everything connected. Nothing in your way.')}
            </Heading>
          </VStack>
          <Link
            to='/pricing'
            className='text-tone-blue-strong inline-flex w-fit shrink-0 items-center gap-2 border-b border-current pb-1 text-sm font-medium'
          >
            {t('Find your next model')}
            <ArrowUpRight className='size-4' aria-hidden='true' />
          </Link>
        </HStack>
        <Grid columns={{ minWidth: 320, max: 2 }} gap={6} align='stretch'>
          <VStack
            as='article'
            gap={8}
            justify='between'
            className='surface-blue rounded-3xl rounded-tr-4xl p-7 md:p-9'
          >
            <VStack gap={5}>
              <HStack justify='between' align='center'>
                <Route className='size-7' aria-hidden='true' />
                <Text type='code' color='inherit'>
                  01 / {t('Model access & routing')}
                </Text>
              </HStack>
              <Heading
                level={3}
                color='inherit'
                className='max-w-sm text-3xl tracking-tight'
              >
                {t('More models. Less plumbing.')}
              </Heading>
              <Text as='p' color='inherit' className='max-w-sm leading-relaxed'>
                {t(
                  'Use familiar API protocols to reach your configured models. Keep your application focused on the experience, not provider integrations.'
                )}
              </Text>
            </VStack>
            <VStack
              gap={3}
              className='py-3'
              aria-label={t('Model access & routing')}
            >
              <HStack
                justify='center'
                align='center'
                gap={2}
                className='surface-brand mx-auto rounded-2xl px-6 py-4'
              >
                <Workflow className='size-5' aria-hidden='true' />
                <Text type='label' color='inherit'>
                  {t('One connection, familiar protocols')}
                </Text>
              </HStack>
              <ArrowDown className='mx-auto size-5' aria-hidden='true' />
              <HStack gap={2} wrap='wrap' justify='center'>
                {['OpenAI', 'Anthropic', 'Gemini'].map((name) => (
                  <Text
                    key={name}
                    type='supporting'
                    color='inherit'
                    className='border-tone-blue-border rounded-full border px-4 py-2'
                  >
                    {name}
                  </Text>
                ))}
              </HStack>
            </VStack>
          </VStack>
          <VStack gap={6}>
            <VStack
              as='article'
              gap={4}
              className='surface-violet rounded-3xl p-7 md:p-8'
            >
              <HStack gap={4} align='center' justify='between'>
                <KeyRound className='size-6' aria-hidden='true' />
                <Text type='code' color='inherit'>
                  02 / {t('Keys & access controls')}
                </Text>
              </HStack>
              <Heading
                level={3}
                color='inherit'
                className='text-2xl tracking-tight'
              >
                {t('Every key has its place.')}
              </Heading>
              <Text as='p' color='inherit' className='leading-relaxed'>
                {t(
                  'Create scoped API keys, set usage limits and manage access from one place. Keep experiments and production neatly separated.'
                )}
              </Text>
            </VStack>
            <VStack
              as='article'
              gap={4}
              className='surface-peach rounded-3xl rounded-bl-4xl p-7 md:p-8'
            >
              <HStack gap={4} align='center' justify='between'>
                <ChartNoAxesCombined className='size-6' aria-hidden='true' />
                <Text type='code' color='inherit'>
                  03 / {t('Usage & observability')}
                </Text>
              </HStack>
              <Heading
                level={3}
                color='inherit'
                className='text-2xl tracking-tight'
              >
                {t('Know where every token goes.')}
              </Heading>
              <Text as='p' color='inherit' className='leading-relaxed'>
                {t(
                  'Bring request logs, model usage and spending into view. Understand your activity before making your next decision.'
                )}
              </Text>
            </VStack>
          </VStack>
        </Grid>
      </VStack>
    </section>
  )
}

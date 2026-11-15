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
  ArrowUpRight,
  ChartNoAxesCombined,
  KeyRound,
  Route,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

interface FeaturesProps {
  className?: string
}

export function Features(props: FeaturesProps) {
  const { t } = useTranslation()
  const features = [
    {
      title: t('Model access & routing'),
      description: t(
        'Use familiar API protocols to reach your configured models. Keep your application focused on the experience, not provider integrations.'
      ),
      icon: Route,
      tone: 'text-tone-blue-strong',
    },
    {
      title: t('Keys & access controls'),
      description: t(
        'Create scoped API keys, set usage limits and manage access from one place. Keep experiments and production neatly separated.'
      ),
      icon: KeyRound,
      tone: 'text-tone-violet-strong',
    },
    {
      title: t('Usage & observability'),
      description: t(
        'Bring request logs, model usage and spending into view. Understand your activity before making your next decision.'
      ),
      icon: ChartNoAxesCombined,
      tone: 'text-tone-teal-strong',
    },
  ]
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
          <Heading
            level={2}
            id='home-features-title'
            type='display-2'
            className='max-w-2xl text-3xl leading-tight tracking-tight text-balance md:text-5xl'
          >
            {t('Everything connected. Nothing in your way.')}
          </Heading>
          <Link
            to='/pricing'
            className='text-tone-blue-strong inline-flex w-fit shrink-0 items-center gap-2 text-sm font-medium underline-offset-4 hover:underline'
          >
            {t('Find your next model')}
            <ArrowUpRight className='size-4' aria-hidden='true' />
          </Link>
        </HStack>
        <Grid columns={{ minWidth: 260, max: 3 }} gap={8} align='start'>
          {features.map((feature) => (
            <VStack
              as='article'
              key={feature.title}
              gap={4}
              className='min-w-0'
            >
              <feature.icon
                className={cn('mb-2 size-7', feature.tone)}
                aria-hidden='true'
              />
              <Heading level={3} className='text-xl tracking-tight'>
                {feature.title}
              </Heading>
              <Text
                as='p'
                color='secondary'
                className='max-w-sm leading-relaxed'
              >
                {feature.description}
              </Text>
            </VStack>
          ))}
        </Grid>
      </VStack>
    </section>
  )
}

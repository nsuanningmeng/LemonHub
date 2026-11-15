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
import { ArrowUpRight, Braces, KeyRound, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function HowItWorks() {
  const { t } = useTranslation()
  const steps = [
    {
      number: '01',
      title: t('Make it your workspace'),
      description: t(
        'Create your account, explore the available models and choose what fits your project.'
      ),
      icon: Sparkles,
      surface: 'surface-violet',
      offset: '',
    },
    {
      number: '02',
      title: t('Make the connection'),
      description: t(
        'Create an API key, point your client to this gateway and send your first request.'
      ),
      icon: KeyRound,
      surface: 'surface-blue',
      offset: 'lg:mt-8',
    },
    {
      number: '03',
      title: t('Make something great'),
      description: t(
        'Follow your usage and costs in the console while you keep building.'
      ),
      icon: Braces,
      surface: 'surface-teal',
      offset: 'lg:mt-16',
    },
  ]
  return (
    <section
      className='px-6 pt-4 pb-16 md:px-10 md:pb-20'
      aria-labelledby='home-workflow-title'
    >
      <VStack gap={10} className='mx-auto max-w-7xl'>
        <VStack gap={3} className='max-w-2xl'>
          <Text
            type='supporting'
            className='text-tone-blue-strong font-mono tracking-widest uppercase'
          >
            {t('From idea to first request')}
          </Text>
          <Heading
            level={2}
            id='home-workflow-title'
            type='display-2'
            className='text-3xl tracking-tight md:text-4xl'
          >
            {t('A small setup. A wide open canvas.')}
          </Heading>
        </VStack>
        <Grid columns={{ minWidth: 260, max: 3 }} gap={6} align='start'>
          {steps.map((step) => (
            <VStack
              as='article'
              key={step.number}
              gap={5}
              className={step.offset}
            >
              <HStack
                justify='between'
                align='center'
                className={`${step.surface} rounded-2xl px-5 py-5`}
              >
                <HStack align='center' gap={4}>
                  <step.icon className='size-6' aria-hidden='true' />
                  <Text type='display-3' color='inherit' className='font-mono'>
                    {step.number}
                  </Text>
                </HStack>
                <ArrowUpRight className='size-6' aria-hidden='true' />
              </HStack>
              <Heading level={3} className='text-lg'>
                {step.title}
              </Heading>
              <Text
                as='p'
                color='secondary'
                className='max-w-sm text-sm leading-relaxed'
              >
                {step.description}
              </Text>
            </VStack>
          ))}
        </Grid>
      </VStack>
    </section>
  )
}

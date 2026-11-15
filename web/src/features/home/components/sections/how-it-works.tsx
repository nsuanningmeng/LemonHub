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
import { Text } from '@astryxdesign/core/Text'
import { VStack } from '@astryxdesign/core/VStack'
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
      tone: 'text-tone-violet-strong',
    },
    {
      number: '02',
      title: t('Make the connection'),
      description: t(
        'Create an API key, point your client to this gateway and send your first request.'
      ),
      tone: 'text-tone-blue-strong',
    },
    {
      number: '03',
      title: t('Make something great'),
      description: t(
        'Follow your usage and costs in the console while you keep building.'
      ),
      tone: 'text-tone-teal-strong',
    },
  ]
  return (
    <section
      className='mx-auto max-w-7xl px-6 pt-4 pb-16 md:px-10 md:pb-20'
      aria-labelledby='home-workflow-title'
    >
      <VStack gap={10}>
        <Heading
          level={2}
          id='home-workflow-title'
          type='display-2'
          className='text-3xl tracking-tight md:text-4xl'
        >
          {t('From idea to first request')}
        </Heading>
        <Grid
          role='list'
          columns={{ minWidth: 260, max: 3 }}
          gap={8}
          align='start'
          className='m-0 list-none p-0'
        >
          {steps.map((step) => (
            <VStack
              role='listitem'
              key={step.number}
              gap={4}
              className='min-w-0'
            >
              <Text type='display-3' className={`${step.tone} font-mono`}>
                {step.number}
              </Text>
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

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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

interface StatsProps {
  className?: string
}

export function Stats(props: StatsProps) {
  const { t } = useTranslation()
  return (
    <section
      aria-label={t('Model ecosystem')}
      className={cn('surface-teal', props.className)}
    >
      <VStack gap={6} className='mx-auto max-w-7xl px-6 py-9 md:px-10'>
        <Text
          type='supporting'
          color='inherit'
          justify='center'
          className='text-sm'
        >
          {t('One workspace for a growing model ecosystem')}
        </Text>
        <HStack
          gap={6}
          wrap='wrap'
          justify='center'
          align='center'
          className='md:justify-between'
        >
          {['OpenAI', 'Anthropic', 'Gemini', 'DeepSeek', 'Qwen', 'Llama'].map(
            (name) => (
              <Text
                key={name}
                type='large'
                color='inherit'
                weight='semibold'
                className='tracking-tight'
              >
                {name}
              </Text>
            )
          )}
        </HStack>
      </VStack>
    </section>
  )
}

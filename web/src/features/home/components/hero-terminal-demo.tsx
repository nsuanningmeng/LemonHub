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
import { Check, Copy, Terminal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { cn } from '@/lib/utils'

const API_EXAMPLES = [
  {
    id: 'openai',
    label: 'OpenAI',
    endpoint: '/v1/chat/completions',
    headers: ['Authorization: Bearer $LEMONHUB_API_KEY'],
    body: {
      model: 'your-model',
      messages: [{ role: 'user', content: 'Hello, world.' }],
    },
  },
  {
    id: 'anthropic',
    label: 'Anthropic',
    endpoint: '/v1/messages',
    headers: ['x-api-key: $LEMONHUB_API_KEY', 'anthropic-version: 2023-06-01'],
    body: {
      model: 'your-model',
      max_tokens: 1024,
      messages: [{ role: 'user', content: 'Hello, world.' }],
    },
  },
  {
    id: 'gemini',
    label: 'Gemini',
    endpoint: '/v1beta/models/your-model:generateContent',
    headers: ['x-goog-api-key: $LEMONHUB_API_KEY'],
    body: { contents: [{ parts: [{ text: 'Hello, world.' }] }] },
  },
] as const

interface HeroTerminalDemoProps {
  className?: string
}

export function HeroTerminalDemo(props: HeroTerminalDemoProps) {
  const { t } = useTranslation()
  const [activeIndex, setActiveIndex] = useState(0)
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const example = API_EXAMPLES[activeIndex]
  const origin =
    typeof window === 'undefined'
      ? 'https://your-gateway.example'
      : window.location.origin
  const requestBody = [
    '{',
    Object.entries(example.body)
      .map(([key, value]) => `  "${key}": ${JSON.stringify(value)}`)
      .join(',\n'),
    '}',
  ].join('\n')
  const command = [
    `curl "${origin}${example.endpoint}"`,
    ...example.headers.map((header) => `  -H "${header}"`),
    '  -H "Content-Type: application/json"',
    `  -d '${requestBody}'`,
  ].join(' \\' + '\n')
  const copied = copiedText === command

  return (
    <VStack
      as='section'
      aria-label={t('Your first request')}
      gap={0}
      className={cn(
        'surface-blue min-w-0 overflow-hidden rounded-2xl',
        props.className
      )}
    >
      <HStack
        gap={2}
        justify='between'
        align='center'
        className='px-5 pt-5 pb-2'
      >
        <HStack
          gap={2}
          align='center'
          className='text-tone-blue-strong text-xs'
        >
          <Terminal className='size-4' aria-hidden='true' />
          <Text type='supporting' color='inherit'>
            {t('Your first request')}
          </Text>
        </HStack>
        <Text type='code' className='text-tone-violet-strong text-xs'>
          cURL
        </Text>
      </HStack>
      <HStack
        gap={2}
        wrap='wrap'
        className='px-5 pt-3'
        aria-label={t('API protocol')}
      >
        {API_EXAMPLES.map((item, index) => (
          <button
            type='button'
            key={item.id}
            aria-pressed={activeIndex === index}
            aria-controls='home-request-example'
            onClick={() => setActiveIndex(index)}
            className={cn(
              'focus-visible:ring-ring rounded-xl px-3 py-2 text-xs transition-colors focus-visible:ring-2 focus-visible:outline-none',
              activeIndex === index
                ? 'surface-brand font-semibold'
                : 'text-muted-foreground hover:bg-tone-blue hover:text-tone-blue-strong'
            )}
          >
            {item.label}
          </button>
        ))}
      </HStack>
      <pre
        id='home-request-example'
        tabIndex={0}
        aria-label={t('Request example')}
        className='text-tone-blue-strong min-h-56 max-w-full overflow-x-auto p-5 font-mono text-xs leading-6 focus-visible:outline-offset-2'
      >
        <code>{command}</code>
      </pre>
      <HStack
        gap={3}
        wrap='wrap'
        justify='between'
        align='center'
        className='px-5 pt-2 pb-5'
      >
        <Text
          as='p'
          type='supporting'
          color='secondary'
          className='min-w-0 flex-1 text-xs leading-relaxed'
        >
          {t('Choose an available model and replace the example key.')}
        </Text>
        <Button
          variant='ghost'
          size='sm'
          className='text-tone-blue-strong hover:bg-tone-blue shrink-0 rounded-xl'
          onClick={() => copyToClipboard(command)}
          aria-label={copied ? t('Copied') : t('Copy request')}
        >
          {copied ? (
            <Check className='size-4' aria-hidden='true' />
          ) : (
            <Copy className='size-4' aria-hidden='true' />
          )}
          {copied ? t('Copied') : t('Copy')}
        </Button>
      </HStack>
    </VStack>
  )
}

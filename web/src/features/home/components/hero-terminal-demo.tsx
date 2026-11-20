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
import { Braces, Terminal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const message = { role: 'user', content: 'Hello!' }

const EXAMPLES = [
  {
    name: 'Chat',
    path: '/v1/chat/completions',
    headers: ['Authorization: Bearer $LEMONHUB_API_KEY'],
    body: (model: string) => ({ model, messages: [message] }),
    response: {
      choices: [{ message: { role: 'assistant', content: 'Hello!' } }],
    },
  },
  {
    name: 'Responses',
    path: '/v1/responses',
    headers: ['Authorization: Bearer $LEMONHUB_API_KEY'],
    body: (model: string) => ({ model, input: 'Hello!' }),
    response: {
      output: [
        { type: 'message', content: [{ type: 'output_text', text: 'Hello!' }] },
      ],
    },
  },
  {
    name: 'Claude',
    path: '/v1/messages',
    headers: ['x-api-key: $LEMONHUB_API_KEY', 'anthropic-version: 2023-06-01'],
    body: (model: string) => ({ model, max_tokens: 128, messages: [message] }),
    response: { content: [{ type: 'text', text: 'Hello!' }] },
  },
  {
    name: 'Gemini',
    path: '/v1beta/models/{model}:generateContent',
    headers: ['x-goog-api-key: $LEMONHUB_API_KEY'],
    body: (_model: string) => ({ contents: [{ parts: [{ text: 'Hello!' }] }] }),
    response: {
      candidates: [{ content: { role: 'model', parts: [{ text: 'Hello!' }] } }],
    },
  },
] as const

function shellLiteral(value: string) {
  return `'${value.replaceAll("'", "'\\''")}'`
}

export function HeroTerminalDemo(props: { serverUrl?: string }) {
  const { t } = useTranslation()
  const [model, setModel] = useState('')
  const [response, setResponse] = useState(false)
  const modelName = model.trim() || 'your-model'
  const serverUrl = props.serverUrl ?? window.location.origin
  const baseUrl = `${serverUrl}/v1`
  return (
    <section className='gateway-examples' aria-label={t('API examples')}>
      <div className='gateway-connection'>
        <div className='gateway-address'>
          <span>{t('Base URL')}</span>
          <div>
            <code>{baseUrl}</code>
            <CopyButton value={baseUrl} aria-label={t('Copy URL')} />
          </div>
        </div>
        <label className='gateway-model-input'>
          <span>{t('Model name')}</span>
          <Input
            value={model}
            onChange={(event) => setModel(event.target.value)}
            placeholder='your-model'
            spellCheck={false}
            autoComplete='off'
          />
        </label>
      </div>
      <Tabs defaultValue='Chat'>
        <TabsList aria-label={t('API examples')} variant='line'>
          {EXAMPLES.map((item) => (
            <TabsTrigger key={item.name} value={item.name}>
              {item.name === 'Chat' ? t('Chat') : item.name}
            </TabsTrigger>
          ))}
        </TabsList>
        {EXAMPLES.map((item) => {
          const path = item.path.replace(
            '{model}',
            encodeURIComponent(modelName)
          )
          const body = JSON.stringify(item.body(modelName), null, 2)
          const command = [
            `curl -X POST ${shellLiteral(serverUrl + path)}`,
            ...item.headers.map((header) => `  -H "${header}"`),
            '  -H "Content-Type: application/json"',
            `  -d ${shellLiteral(body)}`,
          ].join(' \\\n')
          const content = response
            ? JSON.stringify(item.response, null, 2)
            : command
          return (
            <TabsContent value={item.name} key={item.name}>
              <div className='gateway-endpoint'>
                <span>POST</span>
                <code>{path}</code>
              </div>
              <div className='gateway-sample-toolbar'>
                <div className='gateway-sample-views'>
                  <button
                    type='button'
                    aria-pressed={!response}
                    onClick={() => setResponse(false)}
                  >
                    <Terminal size={14} aria-hidden='true' />
                    {t('Request')}
                  </button>
                  <button
                    type='button'
                    aria-pressed={response}
                    onClick={() => setResponse(true)}
                  >
                    <Braces size={14} aria-hidden='true' />
                    {t('Response')}
                  </button>
                </div>
                <CopyButton value={content} aria-label={t('Copy code')} />
              </div>
              <pre
                tabIndex={0}
                aria-label={response ? t('Response') : t('Request')}
              >
                <code>{content}</code>
              </pre>
            </TabsContent>
          )
        })}
      </Tabs>
    </section>
  )
}

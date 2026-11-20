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
import { Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const EXAMPLES = [
  {
    name: 'Chat',
    path: '/v1/chat/completions',
    auth: 'Authorization: Bearer $API_KEY',
    body: '{\n  "model": "your-model",\n  "messages": [{"role": "user", "content": "Hello!"}]\n}',
    response: '{"choices": [{"message": {"content": "Hello!"}}]}',
  },
  {
    name: 'Responses',
    path: '/v1/responses',
    auth: 'Authorization: Bearer $API_KEY',
    body: '{\n  "model": "your-model",\n  "input": "Hello!"\n}',
    response:
      '{"output": [{"type": "message", "content": [{"type": "output_text", "text": "Hello!"}]}]}',
  },
  {
    name: 'Claude',
    path: '/v1/messages',
    auth: 'x-api-key: $API_KEY',
    header: 'anthropic-version: 2023-06-01',
    body: '{\n  "model": "your-model",\n  "max_tokens": 128,\n  "messages": [{"role": "user", "content": "Hello!"}]\n}',
    response: '{"content": [{"type": "text", "text": "Hello!"}]}',
  },
  {
    name: 'Gemini',
    path: '/v1beta/models/{model}:generateContent',
    auth: 'x-goog-api-key: $API_KEY',
    body: '{\n  "contents": [{"parts": [{"text": "Hello!"}]}]\n}',
    response: '{"candidates": [{"content": {"parts": [{"text": "Hello!"}]}}]}',
  },
] as const

export function HeroTerminalDemo() {
  const { t } = useTranslation()
  return (
    <section className='gateway-examples' aria-labelledby='examples-title'>
      <header>
        <Terminal size={17} aria-hidden='true' />
        <h3 id='examples-title'>{t('API examples')}</h3>
      </header>
      <Tabs defaultValue='Chat'>
        <TabsList aria-label={t('API examples')} variant='line'>
          {EXAMPLES.map((item) => (
            <TabsTrigger key={item.name} value={item.name}>
              {item.name === 'Chat' ? t('Chat') : item.name}
            </TabsTrigger>
          ))}
        </TabsList>
        {EXAMPLES.map((item) => (
          <TabsContent value={item.name} key={item.name}>
            <div className='gateway-endpoint'>
              <span>POST</span>
              <code>{item.path}</code>
            </div>
            <div className='gateway-sample'>
              <h4>{t('Request')}</h4>
              <pre>
                <code>
                  <span>{item.auth}</span>
                  {'\nContent-Type: application/json'}
                  {'header' in item && `\n${item.header}`}
                  {'\n\n'}
                  {item.body}
                </code>
              </pre>
            </div>
            <div className='gateway-sample gateway-response'>
              <h4>{t('Response')}</h4>
              <pre>
                <code>{item.response}</code>
              </pre>
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </section>
  )
}

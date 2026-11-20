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
import { BarChart3, BookOpenText, Code2, Languages } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type PlaygroundEmptyStateProps = {
  onSelectPrompt: (prompt: string) => void
}

const starterPrompts = [
  { icon: Code2, text: 'Write a SQL query that counts requests per day' },
  {
    icon: BarChart3,
    text: 'Explain the difference between mean and median latency',
  },
  {
    icon: BookOpenText,
    text: 'Summarize how an API key authenticates a request',
  },
  {
    icon: Languages,
    text: 'Translate a short API error message into three languages',
  },
]

export function PlaygroundEmptyState({
  onSelectPrompt,
}: PlaygroundEmptyStateProps) {
  const { t } = useTranslation()
  return (
    <section className='mx-auto grid w-full max-w-3xl gap-5 py-5 sm:py-10 md:grid-cols-[minmax(0,.65fr)_minmax(0,1fr)] md:items-start md:gap-8'>
      <div>
        <h1 className='text-2xl leading-tight font-semibold tracking-tight md:text-4xl'>
          {t('Model playground')}
        </h1>
        <p className='text-muted-foreground mt-2 max-w-sm text-sm leading-relaxed'>
          {t('Choose a model and send a request to compare its response')}
        </p>
      </div>
      <div>
        <h2 className='text-muted-foreground mb-2 text-sm font-medium'>
          {t('Sample requests')}
        </h2>
        <div className='divide-y border-y'>
          {starterPrompts.map(({ icon: Icon, text }) => (
            <Button
              key={text}
              variant='ghost'
              className='h-auto min-h-11 w-full justify-start gap-3 rounded-none px-1.5 py-2 text-start whitespace-normal'
              onClick={() => onSelectPrompt(t(text))}
            >
              <Icon className='text-primary size-4' aria-hidden='true' />
              {t(text)}
            </Button>
          ))}
        </div>
      </div>
    </section>
  )
}

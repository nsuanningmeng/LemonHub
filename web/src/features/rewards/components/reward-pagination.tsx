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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

interface RewardPaginationProps {
  page: number
  pageSize: number
  total: number
  loading: boolean
  onChange: (page: number) => void
}

export function RewardPagination(props: RewardPaginationProps) {
  const { t } = useTranslation()
  if (props.total <= props.pageSize && props.page === 1) return null
  return (
    <div className='flex flex-wrap items-center justify-between gap-3 pt-4'>
      <p className='text-muted-foreground text-sm'>
        {t('Page {{page}} of {{pages}}', {
          page: props.page,
          pages: Math.max(1, Math.ceil(props.total / props.pageSize)),
        })}
      </p>
      <div className='flex gap-2'>
        <Button
          type='button'
          variant='outline'
          disabled={props.page <= 1 || props.loading}
          onClick={() => props.onChange(props.page - 1)}
        >
          {t('Previous')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={props.page * props.pageSize >= props.total || props.loading}
          onClick={() => props.onChange(props.page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </div>
  )
}

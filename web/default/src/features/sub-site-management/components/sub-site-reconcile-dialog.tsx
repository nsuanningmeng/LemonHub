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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { formatMilliYuan } from '@/components/wallet-logs-table'
import { reconcileSites } from '../api'
import { type ReconcileResult } from '../types'
import { useSubSite } from './sub-site-provider'

export function SubSiteReconcileDialog() {
  const { t } = useTranslation()
  const { open, setOpen } = useSubSite()
  const isOpen = open === 'reconcile'

  const { data, isLoading } = useQuery({
    queryKey: ['sub-site-reconcile', isOpen],
    queryFn: async () => {
      const res = await reconcileSites()
      return res.data ?? []
    },
    enabled: isOpen,
  })

  const results = data ?? []

  const columns: StaticDataTableColumn<ReconcileResult>[] = [
    {
      id: 'site_id',
      header: t('Site ID'),
      cell: (row) => <span className='font-mono text-sm'>{row.site_id}</span>,
    },
    {
      id: 'balance',
      header: t('Balance'),
      cell: (row) => (
        <span className='font-mono text-sm'>¥{formatMilliYuan(row.balance)}</span>
      ),
    },
    {
      id: 'ledger_sum',
      header: t('Ledger Sum'),
      cell: (row) => (
        <span className='font-mono text-sm'>
          ¥{formatMilliYuan(row.ledger_sum)}
        </span>
      ),
    },
    {
      id: 'discrepancy',
      header: t('Discrepancy'),
      cell: (row) => (
        <span
          className={`font-mono text-sm ${row.discrepancy !== 0 ? 'text-destructive' : ''}`}
        >
          ¥{formatMilliYuan(row.discrepancy)}
        </span>
      ),
    },
    {
      id: 'consistent',
      header: t('Consistent'),
      cell: (row) =>
        row.consistent ? (
          <StatusBadge
            label={t('Consistent')}
            variant='success'
            copyable={false}
          />
        ) : (
          <StatusBadge
            label={t('Inconsistent')}
            variant='danger'
            copyable={false}
          />
        ),
    },
  ]

  return (
    <Dialog open={isOpen} onOpenChange={(v) => !v && setOpen(null)}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>{t('Reconciliation')}</DialogTitle>
          <DialogDescription>
            {t(
              'Compare each sub-site wallet balance against its ledger sum. Inconsistent rows are highlighted.'
            )}
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className='flex items-center justify-center py-10'>
            <Spinner />
          </div>
        ) : (
          <StaticDataTable
            columns={columns}
            data={results}
            getRowKey={(row) => row.site_id}
            getRowClassName={(row) =>
              !row.consistent ? 'bg-destructive/10' : undefined
            }
            emptyContent={
              <span className='text-muted-foreground text-sm'>
                {t('No data')}
              </span>
            }
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

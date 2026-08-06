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
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Download, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { DataTablePage, useDataTable } from '@/components/data-table'
import {
  exportRedemptions,
  getRedemptions,
  searchRedemptions,
  voidRedemption,
} from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import { type Redemption } from '../types'
import { GeneratedKeysDialog } from './generated-keys-dialog'
import { RedemptionGenerateDrawer } from './redemption-generate-drawer'
import { useRedemptionColumns } from './redemption-columns'

export function RedemptionTab() {
  const { t } = useTranslation()

  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 })
  const [globalFilter, setGlobalFilter] = useState('')
  const [refreshKey, setRefreshKey] = useState(0)

  const [generateOpen, setGenerateOpen] = useState(false)
  const [generatedKeys, setGeneratedKeys] = useState<string[] | null>(null)
  const [voidTarget, setVoidTarget] = useState<Redemption | null>(null)
  const [isVoiding, setIsVoiding] = useState(false)
  const [isExporting, setIsExporting] = useState(false)

  const columns = useRedemptionColumns((redemption) =>
    setVoidTarget(redemption)
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'site-admin-redemptions',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      refreshKey,
    ],
    queryFn: async () => {
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }
      const result = globalFilter.trim()
        ? await searchRedemptions({ ...params, keyword: globalFilter })
        : await getRedemptions(params)
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    enableRowSelection: false,
    globalFilter,
    onGlobalFilterChange: (updater) => {
      const next =
        typeof updater === 'function' ? updater(globalFilter) : updater
      setGlobalFilter(next)
      setPagination((p) => ({ ...p, pageIndex: 0 }))
    },
    pagination,
    onPaginationChange: setPagination,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
  })

  const handleVoid = async () => {
    if (!voidTarget) return
    setIsVoiding(true)
    try {
      const result = await voidRedemption(voidTarget.id)
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.REDEMPTION_VOIDED))
        setVoidTarget(null)
        setRefreshKey((v) => v + 1)
      }
    } finally {
      setIsVoiding(false)
    }
  }

  const handleExport = async () => {
    setIsExporting(true)
    try {
      const blob = await exportRedemptions()
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `redemptions-${Date.now()}.csv`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
    } catch {
      toast.error(t('Export failed'))
    } finally {
      setIsExporting(false)
    }
  }

  const primaryButtons = (
    <>
      <Button variant='outline' onClick={handleExport} disabled={isExporting}>
        <Download className='h-4 w-4' />
        {t('Export CSV')}
      </Button>
      <Button onClick={() => setGenerateOpen(true)}>
        <Plus className='h-4 w-4' />
        {t('Generate')}
      </Button>
    </>
  )

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Redemption Codes Found')}
        emptyDescription={t(
          'No redemption codes available. Create your first redemption code to get started.'
        )}
        skeletonKeyPrefix='site-admin-redemptions-skeleton'
        applyHeaderSize
        fixedHeight={false}
        paginationInFooter={false}
        toolbarProps={{
          searchPlaceholder: t('Filter by name...'),
          hideViewOptions: true,
          preActions: primaryButtons,
        }}
      />

      <RedemptionGenerateDrawer
        open={generateOpen}
        onOpenChange={setGenerateOpen}
        onGenerated={(keys) => {
          setGeneratedKeys(keys)
          setRefreshKey((v) => v + 1)
        }}
      />

      <GeneratedKeysDialog
        keys={generatedKeys}
        onClose={() => setGeneratedKeys(null)}
      />

      <AlertDialog
        open={voidTarget !== null}
        onOpenChange={(v) => !v && setVoidTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Void Redemption Code')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('This will void the redemption code')}{' '}
              <span className='font-semibold'>{voidTarget?.name}</span>
              {t('. This action cannot be undone.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isVoiding}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleVoid}
              disabled={isVoiding}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {isVoiding ? t('Saving...') : t('Void')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

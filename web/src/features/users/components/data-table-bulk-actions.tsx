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
import { type Table } from '@tanstack/react-table'
import { Power, PowerOff, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { isUserDeleted } from '../constants'
import {
  handleBatchDeleteUsers,
  handleBatchDisableUsers,
  handleBatchEnableUsers,
} from '../lib'
import { type User } from '../types'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [isProcessing, setIsProcessing] = useState(false)

  // Already-deleted users cannot be acted upon; exclude them from any batch.
  // Selection is also gated at the table level (enableRowSelection), so in
  // practice selectedIds matches the toolbar count — this is defense in depth.
  const selectedIds = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original)
    .filter((user) => !isUserDeleted(user))
    .map((user) => user.id)

  const onBatchSuccess = () => {
    triggerRefresh()
    table.resetRowSelection()
  }

  const runBatch = async (batch: () => Promise<void>) => {
    setIsProcessing(true)
    try {
      await batch()
    } finally {
      setIsProcessing(false)
    }
  }

  const handleEnableAll = () =>
    runBatch(() => handleBatchEnableUsers(selectedIds, onBatchSuccess))

  const handleDisableAll = () =>
    runBatch(() => handleBatchDisableUsers(selectedIds, onBatchSuccess))

  const handleDeleteAll = () =>
    runBatch(async () => {
      await handleBatchDeleteUsers(selectedIds, onBatchSuccess)
      setShowDeleteConfirm(false)
    })

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleEnableAll}
                disabled={isProcessing}
                className='size-8'
                aria-label={t('Enable selected users')}
                title={t('Enable selected users')}
              />
            }
          >
            <Power />
            <span className='sr-only'>{t('Enable selected users')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected users')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleDisableAll}
                disabled={isProcessing}
                className='size-8'
                aria-label={t('Disable selected users')}
                title={t('Disable selected users')}
              />
            }
          >
            <PowerOff />
            <span className='sr-only'>{t('Disable selected users')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected users')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => setShowDeleteConfirm(true)}
                disabled={isProcessing}
                className='size-8'
                aria-label={t('Delete selected users')}
                title={t('Delete selected users')}
              />
            }
          >
            <Trash2 />
            <span className='sr-only'>{t('Delete selected users')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Delete selected users')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <ConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={(open) => !open && setShowDeleteConfirm(false)}
        title={t('Delete selected users?')}
        desc={t(
          'This will permanently delete {{count}} selected user(s). This action cannot be undone.',
          { count: selectedIds.length }
        )}
        confirmText={isProcessing ? t('Deleting...') : t('Delete')}
        destructive
        disabled={selectedIds.length === 0}
        isLoading={isProcessing}
        handleConfirm={handleDeleteAll}
      />
    </>
  )
}

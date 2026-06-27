import { Trash2 } from 'lucide-react'
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
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { AdminTicketsTable } from './components/admin/admin-tickets-table'
import { CleanupDialog } from './components/admin/cleanup-dialog'

export function TicketsAdmin() {
  const { t } = useTranslation()
  const [cleanupOpen, setCleanupOpen] = useState(false)

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Ticket Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' onClick={() => setCleanupOpen(true)}>
            <Trash2 className='size-3.5' />
            {t('Cleanup attachments')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <AdminTicketsTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <CleanupDialog open={cleanupOpen} onOpenChange={setCleanupOpen} />
    </>
  )
}

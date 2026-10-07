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
import React, { useState } from 'react'

import useDialogState from '@/hooks/use-dialog'
import { useAuthStore } from '@/stores/auth-store'

import { getPaymentCompliance } from '../api'
import type { PlanRecord, SubscriptionsDialogType } from '../types'

type SubscriptionsContextType = {
  open: SubscriptionsDialogType | null
  setOpen: (str: SubscriptionsDialogType | null) => void
  currentRow: PlanRecord | null
  setCurrentRow: React.Dispatch<React.SetStateAction<PlanRecord | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  complianceConfirmed: boolean
  complianceStatus: 'loading' | 'error' | 'confirmed' | 'unconfirmed'
  retryCompliance: () => void
}

const SubscriptionsContext =
  React.createContext<SubscriptionsContextType | null>(null)

export function SubscriptionsProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [open, setOpen] = useDialogState<SubscriptionsDialogType>(null)
  const [currentRow, setCurrentRow] = useState<PlanRecord | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const capability = useQuery({
    queryKey: ['subscription-payment-compliance', userId],
    queryFn: getPaymentCompliance,
    retry: false,
  })
  let complianceStatus: SubscriptionsContextType['complianceStatus'] = 'loading'
  if (capability.isError) complianceStatus = 'error'
  else if (capability.data) {
    complianceStatus = capability.data.confirmed ? 'confirmed' : 'unconfirmed'
  }
  const complianceConfirmed = complianceStatus === 'confirmed'

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <SubscriptionsContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        complianceConfirmed,
        complianceStatus,
        retryCompliance: () => {
          void capability.refetch()
        },
      }}
    >
      {children}
    </SubscriptionsContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useSubscriptions = () => {
  const ctx = React.useContext(SubscriptionsContext)
  if (!ctx) {
    throw new Error(
      'useSubscriptions has to be used within <SubscriptionsProvider>'
    )
  }
  return ctx
}

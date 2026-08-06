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
import React, { useState } from 'react'
import useDialogState from '@/hooks/use-dialog'
import { type Site, type SubSiteDialogType } from '../types'

type SubSiteContextType = {
  open: SubSiteDialogType | null
  setOpen: (str: SubSiteDialogType | null) => void
  currentRow: Site | null
  setCurrentRow: React.Dispatch<React.SetStateAction<Site | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const SubSiteContext = React.createContext<SubSiteContextType | null>(null)

export function SubSiteProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<SubSiteDialogType>(null)
  const [currentRow, setCurrentRow] = useState<Site | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <SubSiteContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
      }}
    >
      {children}
    </SubSiteContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useSubSite = () => {
  const ctx = React.useContext(SubSiteContext)

  if (!ctx) {
    throw new Error('useSubSite has to be used within <SubSiteProvider>')
  }

  return ctx
}

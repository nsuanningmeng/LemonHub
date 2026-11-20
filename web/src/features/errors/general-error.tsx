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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { ErrorPage } from './error-page'
import {
  claimResourceReload,
  getResourceBuildId,
  isResourceLoadError,
} from './lib/resource-recovery'

type GeneralErrorProps = React.HTMLAttributes<HTMLDivElement> & {
  minimal?: boolean
  error?: unknown
}

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) return undefined
  const response = (error as Record<string, unknown>).response
  if (typeof response !== 'object' || response === null) return undefined
  const status = (response as Record<string, unknown>).status
  return typeof status === 'number' ? status : undefined
}

export function GeneralError({
  className,
  minimal = false,
  error,
}: GeneralErrorProps) {
  const { t } = useTranslation()
  const status = getHttpStatus(error)
  const resourceFailure = isResourceLoadError(error)
  useEffect(() => {
    if (!resourceFailure) return
    const buildId = getResourceBuildId(document)
    if (!buildId) return
    try {
      if (claimResourceReload(window.sessionStorage, buildId)) {
        window.location.reload()
      }
    } catch {
      // Access to sessionStorage itself can be blocked. Keep manual recovery.
    }
  }, [resourceFailure])

  const isRateLimited = status === 429
  let title = isRateLimited
    ? t('Too many requests')
    : t('This page could not be loaded.')
  let description = isRateLimited
    ? t('Please wait a moment before trying again.')
    : t('Please try again later.')
  if (resourceFailure) {
    title = t('Page resources could not be loaded')
    description = t(
      'Check your connection, then reload the page. If the problem continues, contact the administrator.'
    )
  }

  return (
    <ErrorPage
      className={className}
      minimal={minimal}
      code={minimal || resourceFailure ? undefined : (status ?? 500)}
      title={title}
      description={
        <>
          <p>{description}</p>
          {!minimal && !resourceFailure && (
            <p className='mt-3 text-sm'>
              {t(
                'If the problem continues, contact your workspace administrator.'
              )}
            </p>
          )}
        </>
      }
      action={
        resourceFailure && (
          <Button onClick={() => window.location.reload()}>
            {t('Reload page')}
          </Button>
        )
      }
    />
  )
}

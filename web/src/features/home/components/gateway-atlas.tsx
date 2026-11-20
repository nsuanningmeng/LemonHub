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
import { Crosshair, Pause, Play, Waypoints } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'

import { EARTH_FIRST_FRAME } from '../lib/earth-first-frame'

type DeviceHints = Navigator & {
  connection?: { saveData?: boolean }
  deviceMemory?: number
}

function supportsAnimation(): boolean {
  const device = navigator as DeviceHints
  return (
    typeof IntersectionObserver !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    !device.connection?.saveData &&
    (device.deviceMemory ?? 8) > 2 &&
    (device.hardwareConcurrency || 4) > 2
  )
}

export function GatewayAtlas(props: { baseUrl: string }) {
  const { t } = useTranslation()
  const screenRef = useRef<HTMLPreElement>(null)
  const [available, setAvailable] = useState(supportsAnimation)
  const [paused, setPaused] = useState(false)
  const [reduced, setReduced] = useState(
    () =>
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? true
  )

  useEffect(() => {
    if (!available) return
    const query = window.matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => setReduced(query.matches)
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [available])

  useEffect(() => {
    const screen = screenRef.current
    if (!screen || !available || paused || reduced) return

    let cancelled = false
    let stop: (() => void) | undefined
    // The package's player handles offscreen and background-tab suspension.
    // The still frame remains visible while the single-piece chunk loads.
    import('../lib/play-earth')
      .then(({ playEarth }) => {
        if (!cancelled) stop = playEarth(screen)
      })
      .catch(() => {
        if (!cancelled) setAvailable(false)
      })

    return () => {
      cancelled = true
      stop?.()
    }
  }, [available, paused, reduced])

  return (
    <div className='gateway-atlas'>
      <div className='gateway-atlas-heading'>
        <span>
          <Waypoints size={15} aria-hidden='true' />
          {t('API Endpoints')}
        </span>
        {available && !reduced && (
          <button
            type='button'
            className='gateway-motion'
            onClick={() => setPaused(!paused)}
            aria-label={paused ? t('Play animation') : t('Pause animation')}
            title={paused ? t('Play animation') : t('Pause animation')}
          >
            {paused ? <Play size={14} /> : <Pause size={14} />}
          </button>
        )}
      </div>
      <div aria-hidden='true' className='gateway-scope'>
        <i className='gateway-orbit' />
        <Crosshair className='gateway-sight gateway-sight-left' />
        <Crosshair className='gateway-sight gateway-sight-right' />
        <pre ref={screenRef}>{EARTH_FIRST_FRAME}</pre>
      </div>
      <div className='gateway-address'>
        <div className='min-w-0'>
          <span>{t('Base URL')}</span>
          <code>{props.baseUrl}</code>
        </div>
        <CopyButton
          value={props.baseUrl}
          aria-label={t('Copy URL')}
          className='gateway-copy'
        />
      </div>
    </div>
  )
}

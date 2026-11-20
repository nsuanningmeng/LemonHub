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
import { Pause, Play } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import poster from '../assets/earthrise.webp'

type DeviceHints = Navigator & {
  connection?: { saveData?: boolean }
  deviceMemory?: number
}

function supportsAnimation(): boolean {
  const device = navigator as DeviceHints
  return (
    typeof IntersectionObserver !== 'undefined' &&
    typeof OffscreenCanvas !== 'undefined' &&
    typeof Worker !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    !device.connection?.saveData &&
    (device.deviceMemory ?? 8) > 2 &&
    (device.hardwareConcurrency || 4) > 2
  )
}

export function GatewayAtlas() {
  const { t } = useTranslation()
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const lastTime = useRef(0)
  const [available, setAvailable] = useState(supportsAnimation)
  const [paused, setPaused] = useState(false)
  const [ready, setReady] = useState(false)
  const [reduced, setReduced] = useState(
    () =>
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? true
  )

  useEffect(() => {
    if (!available) return
    const media = window.matchMedia('(prefers-reduced-motion: reduce)')
    const change = () => setReduced(media.matches)
    media.addEventListener('change', change)
    return () => media.removeEventListener('change', change)
  }, [available])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !available || reduced || paused) return
    const context = canvas.getContext('bitmaprenderer')
    if (!context) {
      setAvailable(false)
      return
    }
    let visible = false
    let live = true
    let worker: Worker | undefined
    function sync() {
      const play = visible && !document.hidden
      if (!worker && play) {
        try {
          worker = new Worker(
            new URL('../lib/horizon.worker.ts', import.meta.url)
          )
          worker.addEventListener(
            'message',
            (event: MessageEvent<{ bitmap: ImageBitmap; seconds: number }>) => {
              if (!live) {
                event.data.bitmap.close()
                return
              }
              context?.transferFromImageBitmap(event.data.bitmap)
              lastTime.current = event.data.seconds
              setReady(true)
            }
          )
          worker.addEventListener('error', (event) => {
            event.preventDefault()
            if (live) setAvailable(false)
          })
          worker.postMessage({ play, seconds: lastTime.current })
        } catch {
          setAvailable(false)
        }
        return
      }
      worker?.postMessage({ play })
    }
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting
      sync()
    })
    observer.observe(canvas)
    document.addEventListener('visibilitychange', sync)
    return () => {
      live = false
      observer.disconnect()
      document.removeEventListener('visibilitychange', sync)
      worker?.terminate()
    }
  }, [available, paused, reduced])

  return (
    <div className='gateway-panorama'>
      <div className='gateway-landscape' aria-hidden='true'>
        <img
          src={poster}
          alt=''
          width={1400}
          height={700}
          fetchPriority='high'
        />
        <canvas
          ref={canvasRef}
          width={1400}
          height={700}
          data-ready={(ready && available) || undefined}
        />
      </div>
      {ready && available && !reduced && (
        <button
          className='gateway-motion'
          type='button'
          onClick={() => setPaused(!paused)}
          aria-label={paused ? t('Play animation') : t('Pause animation')}
          title={paused ? t('Play animation') : t('Pause animation')}
        >
          {paused ? (
            <Play size={14} aria-hidden='true' />
          ) : (
            <Pause size={14} aria-hidden='true' />
          )}
        </button>
      )}
    </div>
  )
}

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
import earthrise, { meta } from 'ascii.rest/pieces/earthrise'

// Run the official scene away from the UI thread. Each source cell is a dot
// glyph. Cache the few glyph/palette combinations across subsequent frames.
const cell = 7
const paper = '#101a30'
const palette = [
  '#182641',
  '#20304d',
  '#2b405e',
  '#364f72',
  '#46658a',
  '#57799f',
  '#7593b5',
  '#94abc5',
  '#b8c5cf',
  '#d2d8d9',
  '#e5e7de',
  '#f6e6b9',
  ...meta.palette.slice(12),
]
const width = meta.cols * cell
const height = meta.rows * cell
const surface = new OffscreenCanvas(width, height)
const context = surface.getContext('2d')
const colors = new Uint8Array(meta.cols * meta.rows)
const makeFrame = earthrise()
const glyphs = new Map<string, OffscreenCanvas>()
let running = false
let seconds = 0
let timer: ReturnType<typeof setTimeout> | undefined
const rate = 5

function drawFrame() {
  if (!running || !context) return
  const frame = makeFrame(seconds, { color: colors })
  context.fillStyle = paper
  context.fillRect(0, 0, width, height)
  for (let row = 0; row < meta.rows; row++) {
    for (let col = 0; col < meta.cols; col++) {
      const mark = frame[row * (meta.cols + 1) + col]
      if (mark === ' ') continue
      const shade = colors[row * meta.cols + col]
      const key = mark + shade
      let tile = glyphs.get(key)
      if (!tile) {
        tile = new OffscreenCanvas(cell, cell)
        const ink = tile.getContext('2d')
        if (!ink) continue
        ink.font = `${cell * 1.55}px monospace`
        ink.textAlign = 'center'
        ink.textBaseline = 'middle'
        ink.fillStyle = palette[shade]
        ink.fillText(mark, cell / 2, cell / 2)
        glyphs.set(key, tile)
      }
      context.drawImage(tile, col * cell, row * cell)
    }
  }
  const bitmap = surface.transferToImageBitmap()
  self.postMessage({ bitmap, seconds }, { transfer: [bitmap] })
  seconds += 1 / rate
  timer = setTimeout(drawFrame, 1000 / rate)
}

self.addEventListener(
  'message',
  (event: MessageEvent<{ play: boolean; seconds?: number }>) => {
    if (event.data.seconds !== undefined) seconds = event.data.seconds
    if (event.data.play === running) return
    running = event.data.play
    clearTimeout(timer)
    if (running) drawFrame()
  }
)

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
import { beforeEach, describe, expect, test } from 'vitest'

import {
  claimResourceReload,
  getResourceBuildId,
  isResourceLoadError,
} from '../resource-recovery'

beforeEach(() => {
  sessionStorage.clear()
})

describe('resource failure identification', () => {
  test.each([
    {
      name: 'ChunkLoadError',
      message:
        'Loading chunk 63898 failed. (error: /static/js/async/63898.hash.js)',
    },
    {
      code: 'CSS_CHUNK_LOAD_FAILED',
      message: 'Loading CSS chunk 99 failed. (/static/css/99.hash.css)',
    },
    {
      name: 'TypeError',
      message:
        'Failed to fetch dynamically imported module: https://example.test/chunk.js',
    },
    { name: 'TypeError', message: 'Importing a module script failed.' },
    {
      name: 'TypeError',
      message:
        'error loading dynamically imported module: https://example.test/chunk.js',
    },
  ])('recognizes a loader-specific failure: $message', (error) => {
    expect(isResourceLoadError(error)).toBe(true)
  })

  test.each([
    { name: 'TypeError', message: 'Failed to fetch' },
    { name: 'Error', message: 'Unexpected application failure' },
    { name: 'ChunkLoadError', message: 'Unrelated error' },
    {
      name: 'ChunkLoadError',
      message: 'Loading chunk 1 failed.',
      response: { status: 500 },
    },
    {
      name: 'ChunkLoadError',
      message: 'Loading chunk 1 failed.',
      response: { status: 401 },
    },
    { name: 'ChunkLoadError', message: 'Loading chunk 1 failed.', status: 429 },
    null,
  ])(
    'does not reload for an API, authentication or ordinary application error',
    (error) => {
      expect(isResourceLoadError(error)).toBe(false)
    }
  )
})

test('one persisted allowance is shared by routes and chunks for each build in this session', () => {
  expect(claimResourceReload(sessionStorage, 'entry/build-a')).toBe(true)
  expect(claimResourceReload(sessionStorage, 'entry/build-a')).toBe(false)
  expect(claimResourceReload(sessionStorage, 'entry/build-b')).toBe(true)
  expect(claimResourceReload(sessionStorage, 'entry/build-a')).toBe(false)
  expect(claimResourceReload(sessionStorage, '')).toBe(false)
})

test('storage read failure, write failure and silently rejected writes all fail closed', () => {
  expect(
    claimResourceReload(
      {
        getItem: () => {
          throw new Error('blocked')
        },
        setItem: () => undefined,
      },
      'build'
    )
  ).toBe(false)
  expect(
    claimResourceReload(
      {
        getItem: () => null,
        setItem: () => {
          throw new Error('quota')
        },
      },
      'build'
    )
  ).toBe(false)
  expect(
    claimResourceReload(
      { getItem: () => null, setItem: () => undefined },
      'build'
    )
  ).toBe(false)
})

test('build identity comes from the hashed initial entry, ignoring chunk and query differences', () => {
  const doc = document.implementation.createHTMLDocument('fixture')
  const chunk = doc.createElement('script')
  chunk.src = 'https://example.test/static/js/async/1.aaaaaa.js'
  doc.head.append(chunk)
  expect(getResourceBuildId(doc)).toBeUndefined()
  const entry = doc.createElement('script')
  entry.defer = true
  entry.src = 'https://example.test/static/js/index.abcdef1234.js?cache=1'
  doc.head.append(entry)
  expect(getResourceBuildId(doc)).toBe(
    'https://example.test/static/js/index.abcdef1234.js'
  )
  entry.src = 'https://example.test/static/js/index.abcdef1234.js?cache=2'
  expect(getResourceBuildId(doc)).toBe(
    'https://example.test/static/js/index.abcdef1234.js'
  )
  entry.src = 'https://example.test/src/main.tsx'
  expect(getResourceBuildId(doc)).toBeUndefined()
})

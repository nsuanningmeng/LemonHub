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
import { api } from '@/lib/api'

// Use the relay's token authorization rather than reconstructing group and
// model-limit rules in the dashboard. Never share GET requests across keys.
export async function getCCSwitchModels(
  tokenKey: string,
  signal: AbortSignal
): Promise<string[]> {
  const key = tokenKey.startsWith('sk-') ? tokenKey : `sk-${tokenKey}`
  const response = await api.get<{ data?: { id: string }[] }>('/v1/models', {
    headers: { 'x-api-key': key },
    signal,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    disableDuplicate: true,
  })
  if (!Array.isArray(response.data?.data)) {
    throw new Error(
      'Unable to load models for this API key. Check its permissions and availability.'
    )
  }
  return [
    ...new Set(
      response.data.data
        .map((model) => model.id)
        .filter((id) => typeof id === 'string' && id.length > 0)
    ),
  ]
}

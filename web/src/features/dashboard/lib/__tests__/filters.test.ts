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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DASHBOARD_CHART_PREFERENCES_STORAGE_KEY } from '../../constants'
import { buildDefaultDashboardFilters } from '../filters'

describe('default dashboard filters', () => {
  beforeEach(() => {
    localStorage.removeItem(DASHBOARD_CHART_PREFERENCES_STORAGE_KEY)
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-11-24T08:30:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
    localStorage.removeItem(DASHBOARD_CHART_PREFERENCES_STORAGE_KEY)
  })

  it('queries the previous seven days when no range preference is saved', () => {
    expect(buildDefaultDashboardFilters()).toMatchObject({
      start_timestamp: new Date('2026-11-17T08:30:00Z'),
      end_timestamp: new Date('2026-11-24T08:30:00Z'),
    })
  })

  it('honors an explicitly saved one-day range', () => {
    localStorage.setItem(
      DASHBOARD_CHART_PREFERENCES_STORAGE_KEY,
      JSON.stringify({ defaultTimeRangeDays: 1 })
    )

    expect(buildDefaultDashboardFilters()).toMatchObject({
      start_timestamp: new Date('2026-11-23T08:30:00Z'),
      end_timestamp: new Date('2026-11-24T08:30:00Z'),
    })
  })
})

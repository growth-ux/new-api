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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Footer } from '../footer'

let client: QueryClient
let status: Record<string, unknown>

beforeEach(() => {
  localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  status = {
    system_name: 'Team AI',
    user_agreement_enabled: false,
    privacy_policy_enabled: false,
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return { data: { success: true, data: status } }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

async function renderFooter() {
  const router = createRouter({
    routeTree: createRootRoute({ component: Footer }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('home footer content', () => {
  it('keeps the platform brand without copyright or project attribution', async () => {
    await renderFooter()
    expect(
      await screen.findByRole('img', { name: 'Team AI' })
    ).toBeInTheDocument()
    expect(screen.queryByText(/©/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'New API' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'User Agreement' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Privacy Policy' })
    ).not.toBeInTheDocument()
  })

  it('keeps enabled policy links in the default footer', async () => {
    status.user_agreement_enabled = true
    status.privacy_policy_enabled = true
    await renderFooter()
    expect(
      await screen.findByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    expect(
      screen.getByRole('link', { name: 'Privacy Policy' })
    ).toHaveAttribute('href', '/privacy-policy')
  })

  it('preserves custom content and policy links without appending project attribution', async () => {
    status.footer_html = '<p>Platform support information</p>'
    status.user_agreement_enabled = true
    status.privacy_policy_enabled = true
    await renderFooter()
    expect(
      await screen.findByText('Platform support information')
    ).toBeVisible()
    expect(
      screen.getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    expect(
      screen.getByRole('link', { name: 'Privacy Policy' })
    ).toHaveAttribute('href', '/privacy-policy')
    expect(
      screen.queryByRole('link', { name: 'New API' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText(/©/)).not.toBeInTheDocument()
  })
})

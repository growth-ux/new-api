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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { About } from '../index'

let client: QueryClient
let content: string

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  content = ''
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/about') {
      return { data: { success: true, data: content } }
    }
    if (url === '/api/status') {
      return {
        data: {
          success: true,
          data: { system_name: 'Team AI', docs_link: '/docs' },
        },
      }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

async function renderAbout() {
  const router = createRouter({
    routeTree: createRootRoute({ component: About }),
    history: createMemoryHistory({ initialEntries: ['/about'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('platform introduction', () => {
  it('shows the configured brand and local documentation when no introduction is configured', async () => {
    await renderAbout()
    expect(
      await screen.findByRole('heading', { name: 'Team AI' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Legal notices' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Original project' })
    ).not.toBeInTheDocument()
    expect(
      screen
        .getAllByRole('link', { name: 'Docs' })
        .every((link) => link.getAttribute('href') === '/docs')
    ).toBe(true)
  })

  it('preserves custom Markdown content', async () => {
    content = '# Our platform\nCustom platform information.'
    await renderAbout()
    expect(
      await screen.findByRole('heading', { name: 'Our platform' })
    ).toBeVisible()
  })

  it('preserves an embedded introduction', async () => {
    content = 'https://example.com/introduction'
    await renderAbout()
    await waitFor(() =>
      expect(document.querySelector('iframe')).toHaveAttribute('src', content)
    )
    expect(document.querySelector('iframe')).toHaveAttribute(
      'sandbox',
      'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
    )
  })
})

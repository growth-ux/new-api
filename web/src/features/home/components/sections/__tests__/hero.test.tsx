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

import { Hero } from '../hero'

let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { docs_link: '/docs' } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  localStorage.clear()
})

describe('home hero actions', () => {
  it.each([false, true])(
    'keeps primary actions and documentation without app promotion when authenticated=%s',
    async (authenticated) => {
      const router = createRouter({
        routeTree: createRootRoute({
          component: () => <Hero isAuthenticated={authenticated} />,
        }),
        history: createMemoryHistory({ initialEntries: ['/'] }),
      })
      await router.load()
      render(
        <QueryClientProvider client={client}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      )
      expect(
        await screen.findByRole('button', {
          name: authenticated ? 'Go to Dashboard' : 'Get Started',
        })
      ).toHaveAttribute('href', authenticated ? '/dashboard' : '/sign-up')
      expect(screen.getByRole('button', { name: 'Docs' })).toHaveAttribute(
        'href',
        '/docs'
      )
      expect(
        screen.queryByText('Supported Applications')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Cherry Studio')).not.toBeInTheDocument()
      expect(screen.queryByText('CC Switch')).not.toBeInTheDocument()
      expect(screen.queryByText('More Apps')).not.toBeInTheDocument()
    }
  )
})

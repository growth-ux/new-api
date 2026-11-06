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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function Docs() {
  const { t } = useTranslation()
  const baseUrl = `${window.location.origin}/v1`
  const steps = [
    {
      title: t('Create an API key'),
      description: t(
        'Sign in and create an API key. Keep it private and use it only in trusted applications.'
      ),
      href: '/keys',
      action: t('API Keys'),
    },
    {
      title: t('Choose a model'),
      description: t(
        'Check available models and pricing, then use the exact model ID in your requests.'
      ),
      href: '/pricing',
      action: t('Model Square'),
    },
    {
      title: t('Send your first request'),
      description: t(
        'Set the API base URL below and your API key in an OpenAI-compatible client, or try a model in the playground.'
      ),
      href: '/playground',
      action: t('Playground'),
    },
  ]

  return (
    <PublicLayout>
      <div className='mx-auto max-w-4xl space-y-8 px-2 py-8 sm:px-4'>
        <div className='space-y-3'>
          <h1 className='text-3xl font-semibold tracking-tight'>{t('Docs')}</h1>
          <p className='text-muted-foreground'>
            {t('Connect your application in three steps.')}
          </p>
        </div>
        <div className='grid gap-4 md:grid-cols-3'>
          {steps.map((step, index) => (
            <Card key={step.href}>
              <CardHeader>
                <CardTitle>
                  <h2>
                    {index + 1}. {step.title}
                  </h2>
                </CardTitle>
              </CardHeader>
              <CardContent className='flex flex-1 flex-col items-start gap-5'>
                <p className='text-muted-foreground flex-1 leading-6'>
                  {step.description}
                </p>
                <Button variant='outline' render={<Link to={step.href} />}>
                  {step.action}
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
        <Card>
          <CardHeader>
            <CardTitle>
              <h2>{t('API Base URL')}</h2>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className='bg-muted flex items-center gap-3 rounded-lg p-3'>
              <code className='min-w-0 flex-1 break-all'>{baseUrl}</code>
              <CopyButton value={baseUrl} aria-label={t('Copy API base URL')} />
            </div>
            <p className='text-muted-foreground mt-4 text-sm leading-6'>
              {t(
                'View request usage in Usage Logs. For access or billing questions, contact your platform administrator.'
              )}
            </p>
          </CardContent>
        </Card>
      </div>
    </PublicLayout>
  )
}

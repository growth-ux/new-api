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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getAboutContent } from './api'

function PlatformIntroduction() {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()

  return (
    <section className='space-y-5 py-8'>
      <h1 className='text-3xl font-semibold tracking-tight'>{systemName}</h1>
      <p className='text-muted-foreground max-w-2xl leading-7'>
        {t(
          'One platform for model access, API keys, usage tracking, and team management.'
        )}
      </p>
      <Button render={<Link to='/docs' />}>{t('Docs')}</Button>
    </section>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['about-content'],
    queryFn: async () => requireServerSuccess(await getAboutContent()),
  })
  const content = data?.data?.trim() ?? ''

  let introduction = <PlatformIntroduction />
  if (isLoading) {
    introduction = (
      <div className='space-y-4 py-8'>
        <Skeleton className='h-8 w-[45%]' />
        <Skeleton className='h-4 w-full' />
        <Skeleton className='h-4 w-[80%]' />
      </div>
    )
  } else if (content && isHttpUrl(content)) {
    introduction = (
      <iframe
        src={content}
        className='h-[65vh] w-full border-0'
        title={t('About')}
        sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
      />
    )
  } else if (content && isLikelyHtml(content)) {
    introduction = (
      <RichContent mode='html' htmlVariant='isolated' content={content} />
    )
  } else if (content) {
    introduction = <RichContent mode='markdown' content={content} />
  }

  return (
    <PublicLayout>
      <div className='mx-auto max-w-4xl px-2 py-8 sm:px-4'>{introduction}</div>
    </PublicLayout>
  )
}

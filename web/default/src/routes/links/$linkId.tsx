/* eslint-disable react/iframe-missing-sandbox -- External forms need scripts and their own origin; same-origin targets are excluded. */
import { createFileRoute } from '@tanstack/react-router'
import { ExternalLink, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { useStatus } from '@/hooks/use-status'
import { parseCustomNavigationLinks } from '@/lib/custom-navigation'

export const Route = createFileRoute('/links/$linkId')({
  component: CustomLinkPage,
})

function CustomLinkPage() {
  const { linkId } = Route.useParams()
  const { status, loading } = useStatus()
  const { t } = useTranslation()
  const link = parseCustomNavigationLinks(status?.CustomNavigationLinks).find(
    (item) => item.id === linkId && (item.header || item.sidebar || item.home)
  )
  const canEmbed =
    link?.mode === 'iframe' &&
    new URL(link.url).origin !== window.location.origin
  const [frameLoading, setFrameLoading] = useState(true)
  useEffect(() => {
    setFrameLoading(true)
    const timer = window.setTimeout(() => setFrameLoading(false), 10000)
    return () => window.clearTimeout(timer)
  }, [link?.url])
  return (
    <PublicLayout>
      <main className='mx-auto w-full max-w-screen-2xl px-4 py-4'>
        {link ? (
          <>
            <div className='mb-4 flex flex-wrap items-center justify-between gap-3'>
              <h1 className='min-w-0 text-xl font-semibold break-words'>
                {link.title}
              </h1>
              {canEmbed && frameLoading && (
                <Loader2
                  className='size-4 animate-spin'
                  role='status'
                  aria-label={t('Loading...')}
                />
              )}
              <a
                className='inline-flex items-center gap-2 text-sm underline'
                href={link.url}
                target='_blank'
                rel='noopener noreferrer'
              >
                <ExternalLink className='size-4' />
                {t('Open in new tab')}
              </a>
            </div>
            {/* External forms need their own origin for storage and scripts; same-origin targets are excluded above. */}
            {canEmbed && (
              <iframe
                key={link.url}
                title={link.title}
                src={link.url}
                onLoad={() => setFrameLoading(false)}
                className='h-[75vh] min-h-96 w-full border-0 bg-white'
                sandbox='allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads'
                referrerPolicy='no-referrer'
              />
            )}
          </>
        ) : (
          <p>{loading ? t('Loading...') : t('Not found')}</p>
        )}
      </main>
    </PublicLayout>
  )
}

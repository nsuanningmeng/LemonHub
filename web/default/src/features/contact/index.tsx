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
import { Mail, Handshake, MessagesSquare, Clock } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Markdown } from '@/components/ui/markdown'
import { Skeleton } from '@/components/ui/skeleton'
import { AnimateInView } from '@/components/animate-in-view'
import { Footer } from '@/components/layout/components/footer'
import { PublicLayout } from '@/components/layout'
import { getContactContent } from './api'

function isValidUrl(value: string) {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function isLikelyHtml(value: string) {
  return /<\/?[a-z][\s\S]*>/i.test(value)
}

// NOTE(替换): 以下联系方式为占位（仅在管理员未在「系统设置 → 站点信息 → 联系我们」
// 配置内容时作为默认页展示）。请替换为真实的邮箱 / 微信 / Telegram / QQ 群等。
// value 为实际展示的联系信息（不走 i18n，按需直接改）；href 可留空或填 mailto:/外链。
const CONTACT_METHODS = [
  {
    icon: Mail,
    title: 'Email',
    desc: 'For general questions and technical support',
    value: 'support@example.com',
    href: 'mailto:support@example.com',
  },
  {
    icon: Handshake,
    title: 'Affiliate & Business',
    desc: 'Apply to become an agent, or discuss partnerships',
    value: 'WeChat: your-wechat-id',
    href: '',
  },
  {
    icon: MessagesSquare,
    title: 'Community & Online Support',
    desc: 'Join our group for real-time help',
    value: 'Telegram / QQ Group',
    href: '',
  },
]

function ContactMethodCard({
  icon: Icon,
  title,
  desc,
  value,
  href,
  delay,
}: {
  icon: typeof Mail
  title: string
  desc: string
  value: string
  href: string
  delay: number
}) {
  const { t } = useTranslation()
  const inner = (
    <>
      <span className='border-border/50 bg-muted/30 text-foreground mb-5 flex size-12 items-center justify-center rounded-xl border'>
        <Icon className='size-5' strokeWidth={1.75} />
      </span>
      <h3 className='mb-1.5 text-base font-semibold'>{t(title)}</h3>
      <p className='text-muted-foreground mb-4 text-sm leading-relaxed'>
        {t(desc)}
      </p>
      <span className='text-foreground/90 mt-auto text-sm font-medium break-all'>
        {value}
      </span>
    </>
  )
  const cls =
    'border-border/50 bg-card hover:border-border hover:bg-muted/30 flex flex-col rounded-2xl border p-6 transition-all duration-300 hover:-translate-y-1'
  return (
    <AnimateInView delay={delay} animation='fade-up'>
      {href ? (
        <a href={href} className={cls}>
          {inner}
        </a>
      ) : (
        <div className={cls}>{inner}</div>
      )}
    </AnimateInView>
  )
}

// 默认联系页（管理员未在系统设置配置内容时展示）
function DefaultContactPage() {
  const { t } = useTranslation()
  return (
    <main className='overflow-x-hidden'>
      <section className='relative z-10 overflow-hidden px-6 pt-24 pb-24 md:pt-32 md:pb-32'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 -z-10 opacity-25 dark:opacity-[0.12]'
          style={{
            background:
              'radial-gradient(ellipse 60% 50% at 50% 0%, oklch(0.72 0.18 250 / 70%) 0%, transparent 70%)',
          }}
        />
        <div
          aria-hidden
          className='absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_20%,black_20%,transparent_100%)] bg-[size:4rem_4rem] opacity-[0.08]'
        />

        <div className='mx-auto max-w-5xl'>
          <AnimateInView className='mb-14 text-center md:mb-16'>
            <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
              {t('Get in Touch')}
            </p>
            <h1 className='text-[clamp(2rem,4vw,2.75rem)] leading-[1.15] font-bold tracking-tight'>
              {t('Contact Us')}
            </h1>
            <p className='text-muted-foreground/80 mx-auto mt-4 max-w-2xl text-sm leading-relaxed md:text-base'>
              {t(
                'Questions about the affiliate program, business cooperation, or support — reach us through any channel below.'
              )}
            </p>
          </AnimateInView>

          <div className='grid gap-6 sm:grid-cols-2 lg:grid-cols-3'>
            {CONTACT_METHODS.map((m, i) => (
              <ContactMethodCard key={m.title} {...m} delay={i * 100} />
            ))}
          </div>

          <AnimateInView delay={300} className='mt-10 flex justify-center'>
            <div className='border-border/50 bg-muted/20 text-muted-foreground flex items-center gap-2 rounded-full border px-4 py-2 text-sm'>
              <Clock className='size-4' strokeWidth={1.75} />
              {t('We typically respond within 1 business day.')}
            </div>
          </AnimateInView>
        </div>
      </section>
      <Footer />
    </main>
  )
}

export function Contact() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['contact-content'],
    queryFn: getContactContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isValidUrl(rawContent)
  const isHtml = hasContent && !isUrl && isLikelyHtml(rawContent)

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  // Admin-configured content (set in System Settings → Site → Contact)
  if (hasContent) {
    if (isUrl) {
      return (
        <PublicLayout showMainContainer={false}>
          <iframe
            src={rawContent}
            className='h-[calc(100vh-3.5rem)] w-full border-0'
            title={t('Contact Us')}
          />
        </PublicLayout>
      )
    }
    return (
      <PublicLayout>
        <div className='mx-auto max-w-6xl px-4 py-8'>
          {isHtml ? (
            <div
              className='prose prose-neutral dark:prose-invert max-w-none'
              dangerouslySetInnerHTML={{ __html: rawContent }}
            />
          ) : (
            <Markdown className='prose-neutral dark:prose-invert max-w-none'>
              {rawContent}
            </Markdown>
          )}
        </div>
      </PublicLayout>
    )
  }

  // Default styled contact page
  return (
    <PublicLayout showMainContainer={false}>
      <DefaultContactPage />
    </PublicLayout>
  )
}

export default Contact

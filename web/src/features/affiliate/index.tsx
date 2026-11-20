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
import { ArrowRight, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { buttonVariants } from '@/components/ui/button'

import {
  ADVANTAGES,
  AUDIENCES,
  FAQS,
  HERO_SELLING_POINTS,
  REFERRAL_TIERS,
  REFERRAL_VS_AGENT,
  STEPS,
  WHOLESALE_TIERS,
} from './content'

function AffiliateSection(props: {
  id?: string
  title: string
  description?: string
  children: React.ReactNode
}) {
  return (
    <section
      id={props.id}
      tabIndex={props.id ? -1 : undefined}
      className='scroll-mt-20 border-t py-10 sm:py-14'
    >
      <div className='grid items-start gap-7 lg:grid-cols-[minmax(14rem,0.7fr)_minmax(0,2fr)] lg:gap-14'>
        <header>
          <h2 className='text-2xl leading-tight font-semibold tracking-tight text-balance md:text-3xl'>
            {props.title}
          </h2>
          {props.description && (
            <p className='text-muted-foreground mt-3 max-w-prose text-sm leading-relaxed'>
              {props.description}
            </p>
          )}
        </header>
        {props.children}
      </div>
    </section>
  )
}

function AffiliateHero() {
  const { t } = useTranslation()
  return (
    <header className='grid items-end gap-10 pt-28 pb-12 md:pt-36 md:pb-16 lg:grid-cols-[1.2fr_1fr] lg:gap-20'>
      <div>
        <p className='text-primary mb-3 text-sm font-medium'>
          {t('Affiliate Program · Now Recruiting')}
        </p>
        <h1 className='max-w-2xl text-4xl leading-[1.04] font-semibold tracking-tight text-balance sm:text-6xl'>
          {t('Become Our Exclusive Agent')}
        </h1>
        <p className='text-muted-foreground mt-5 max-w-xl text-base leading-relaxed'>
          {t(
            'With just a domain, you can own an AI site under your own brand. Stock at a wholesale discount, set your own retail price, and keep the full margin — user payments land directly in your own account, while the platform handles the tech and models.'
          )}
        </p>
        <div className='mt-6 flex flex-wrap items-center gap-3'>
          <Link to='/contact' className={buttonVariants({ size: 'lg' })}>
            {t('Apply Now')}
            <ArrowRight aria-hidden='true' />
          </Link>
          <a
            href='#affiliate-advantages'
            className={buttonVariants({ size: 'lg', variant: 'outline' })}
          >
            {t('Learn More')}
          </a>
        </div>
      </div>
      <ul className='border-primary/60 divide-border divide-y border-t-2'>
        {HERO_SELLING_POINTS.map(({ icon: Icon, text }) => (
          <li key={text} className='flex items-start gap-4 py-5'>
            <Icon
              aria-hidden='true'
              className='text-primary mt-0.5 size-5 shrink-0'
              strokeWidth={1.65}
            />
            <span className='text-base leading-relaxed'>{t(text)}</span>
          </li>
        ))}
      </ul>
    </header>
  )
}

export function Affiliate() {
  const { t } = useTranslation()
  return (
    <PublicLayout showMainContainer={false}>
      <main className='mx-auto w-full max-w-[76rem] px-5 sm:px-8'>
        <AffiliateHero />
        <AffiliateSection title={t('Who It Is For')}>
          <div className='grid gap-x-8 gap-y-7 sm:grid-cols-2'>
            {AUDIENCES.map(({ icon: Icon, name, desc }) => (
              <article key={name}>
                <h3 className='flex items-baseline gap-2 text-base font-semibold'>
                  <Icon
                    aria-hidden='true'
                    className='text-muted-foreground size-4 shrink-0 self-center'
                  />
                  {t(name)}
                </h3>
                <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                  {t(desc)}
                </p>
              </article>
            ))}
          </div>
        </AffiliateSection>
        <AffiliateSection
          id='affiliate-advantages'
          title={t('Affiliate Advantages')}
          description={t(
            'The platform handles the tech and model ecosystem — you focus on brand operations and growth'
          )}
        >
          <dl className='divide-border divide-y'>
            {ADVANTAGES.map(({ title, desc }) => (
              <div
                key={title}
                className='grid gap-1.5 py-4 first:pt-0 sm:grid-cols-[0.8fr_1.4fr] sm:gap-5'
              >
                <dt className='text-sm font-semibold'>{t(title)}</dt>
                <dd className='text-muted-foreground text-sm leading-relaxed'>
                  {t(desc)}
                </dd>
              </div>
            ))}
          </dl>
        </AffiliateSection>
        <AffiliateSection title={t('Start in Four Steps')}>
          <ol className='grid gap-6 sm:grid-cols-2'>
            {STEPS.map(({ title, desc }, index) => (
              <li key={title} className='flex items-start gap-3'>
                <span
                  aria-hidden='true'
                  className='bg-accent text-accent-foreground grid size-7 shrink-0 place-items-center rounded-md font-mono text-sm'
                >
                  {index + 1}
                </span>
                <div>
                  <h3 className='text-base font-semibold'>{t(title)}</h3>
                  <p className='text-muted-foreground mt-1.5 text-sm leading-relaxed'>
                    {t(desc)}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </AffiliateSection>
        <AffiliateSection
          title={t('Wholesale Discount Tiers')}
          description={t(
            'The lower your wholesale rate, the higher your margin — the tiers below are examples, final terms are subject to the actual agreement.'
          )}
        >
          <div>
            <dl className='divide-border divide-y border-y'>
              {WHOLESALE_TIERS.map(({ tier, rate, note, recommended }) => (
                <div
                  key={tier}
                  className='grid grid-cols-[1fr_auto] items-baseline gap-x-4 gap-y-1 py-4 sm:grid-cols-[6rem_minmax(0,1fr)_auto]'
                >
                  <dt className='font-medium'>
                    {t(tier)}
                    {recommended && (
                      <span className='text-primary block text-xs font-medium'>
                        {t('Recommended')}
                      </span>
                    )}
                  </dt>
                  <dd className='text-muted-foreground col-start-1 row-start-2 text-sm sm:col-auto sm:row-auto'>
                    {t(note)}
                  </dd>
                  <dd className='col-start-2 row-start-1 text-end sm:col-auto sm:row-auto'>
                    <span className='font-mono text-lg font-medium tabular-nums'>
                      {rate}
                    </span>{' '}
                    <span className='text-muted-foreground text-xs'>
                      {t('of retail')}
                    </span>
                  </dd>
                </div>
              ))}
            </dl>
            <p className='text-muted-foreground mt-3 text-sm'>
              {t('You stock at this rate; the full margin is yours.')}
            </p>
          </div>
        </AffiliateSection>
        <AffiliateSection
          title={t('Referral Rewards Program')}
          description={t(
            'No site to build. Share your personal invite link, and earn commission whenever the users you bring in top up on this platform. This is a separate path from the Brand Agent program above — choose either, they do not affect each other.'
          )}
        >
          <div>
            <div className='grid gap-8 sm:grid-cols-2'>
              {REFERRAL_TIERS.map(({ icon: Icon, title, desc, points }) => (
                <article key={title}>
                  <Icon
                    aria-hidden='true'
                    className='text-primary mb-3 size-5'
                  />
                  <h3 className='text-lg leading-snug font-semibold'>
                    {t(title)}
                  </h3>
                  <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                    {t(desc)}
                  </p>
                  <ul className='mt-4 space-y-2'>
                    {points.map((point) => (
                      <li
                        key={point}
                        className='flex gap-2 text-sm leading-relaxed'
                      >
                        <Check
                          aria-hidden='true'
                          className='text-primary mt-1 size-3.5 shrink-0'
                        />
                        {t(point)}
                      </li>
                    ))}
                  </ul>
                </article>
              ))}
            </div>
            <aside className='bg-muted/60 mt-7 rounded-lg p-5'>
              <h3 className='text-base font-semibold'>
                {t('How is this different from the Brand Agent program?')}
              </h3>
              <dl className='mt-3 space-y-2'>
                {REFERRAL_VS_AGENT.map(({ label, desc }) => (
                  <div key={label} className='text-sm leading-relaxed'>
                    <dt className='inline font-semibold'>{t(label)} </dt>
                    <dd className='text-muted-foreground inline'>{t(desc)}</dd>
                  </div>
                ))}
              </dl>
              <p className='text-muted-foreground mt-3 text-sm'>
                {t('The two paths are independent and can run in parallel.')}
              </p>
            </aside>
            <div className='mt-6 flex flex-wrap gap-3'>
              <Link to='/referral' className={buttonVariants()}>
                {t('Get my referral link')}
                <ArrowRight aria-hidden='true' />
              </Link>
              <Link
                to='/contact'
                className={buttonVariants({ variant: 'outline' })}
              >
                {t('Apply as a Professional Promoter')}
              </Link>
            </div>
          </div>
        </AffiliateSection>
        <AffiliateSection title={t('Frequently Asked Questions')}>
          <Accordion>
            {FAQS.map((item, index) => (
              <AccordionItem key={item.q} value={String(index)}>
                <AccordionTrigger className='py-4 text-start text-base'>
                  {t(item.q)}
                </AccordionTrigger>
                <AccordionContent className='text-muted-foreground max-w-prose pb-4 text-sm leading-relaxed'>
                  {t(item.a)}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </AffiliateSection>
        <section
          id='affiliate-apply'
          tabIndex={-1}
          className='flex scroll-mt-20 flex-col items-start gap-5 border-t py-12 md:flex-row md:items-center md:justify-between'
        >
          <div>
            <h2 className='text-2xl font-semibold tracking-tight'>
              {t('Ready to start your journey as an agent?')}
            </h2>
            <p className='text-muted-foreground mt-2 max-w-prose leading-relaxed'>
              {t(
                'One domain, one AI site under your own brand — set your price and start earning today.'
              )}
            </p>
          </div>
          <Link to='/contact' className={buttonVariants({ size: 'lg' })}>
            {t('Apply Now')}
            <ArrowRight aria-hidden='true' />
          </Link>
        </section>
      </main>
      <Footer />
    </PublicLayout>
  )
}

export default Affiliate

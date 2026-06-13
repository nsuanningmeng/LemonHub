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
import {
  ArrowRight,
  Globe,
  Wallet,
  Rocket,
  Palette,
  Headphones,
  Boxes,
  Code2,
  Megaphone,
  Users2,
  Store,
  FileText,
  Server,
  Coins,
  CheckCircle2,
} from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import { Footer } from '@/components/layout/components/footer'
import { PublicLayout } from '@/components/layout'

// "立即申请" 跳转到「联系我们」页（/contact）。如后续有专门的代理申请表单页，
// 把下方两个 Button 的 `<Link to='/contact' />` 改成对应路由即可。

// ---------------------------------------------------------------------------
// 数据（文案为 i18n key = 英文源串，zh.json 提供中文翻译；图标用 lucide-react）
// ---------------------------------------------------------------------------

const HERO_SELLING_POINTS = [
  {
    icon: Globe,
    text: 'Just provide a domain and go live by following the guide',
  },
  {
    icon: Palette,
    text: 'Freely customize pricing, homepage, notices and guides',
  },
  {
    icon: Wallet,
    text: 'Earn a share from user online top-ups and card-key sales',
  },
]

const AUDIENCES = [
  {
    icon: Code2,
    name: 'Developers / Studios',
    desc: 'Already have the skills and an audience — quickly launch your own branded AI site to monetize.',
  },
  {
    icon: Megaphone,
    name: 'Influencers / KOLs',
    desc: 'Turn your fan traffic into your own paying users and earn an ongoing share of their top-ups.',
  },
  {
    icon: Users2,
    name: 'Startup Teams',
    desc: 'Plug into a complete AI gateway with zero R&D cost and focus on operations and growth.',
  },
  {
    icon: Store,
    name: 'Resellers / Agents',
    desc: 'Batch-generate card keys, connect to card platforms, and distribute at scale.',
  },
]

const ADVANTAGES = [
  {
    icon: Rocket,
    title: 'One-click Deployment',
    desc: 'A single deployment and one database can host multiple independent branded sub-sites — no development needed.',
  },
  {
    icon: Globe,
    title: 'Independent Domains',
    desc: 'Each sub-site binds its own domain and automatically shows its dedicated brand by domain.',
  },
  {
    icon: Palette,
    title: 'Custom Homepage & Pricing',
    desc: 'Site name, logo, notices, footer and pricing discounts are all self-service configurable.',
  },
  {
    icon: Coins,
    title: 'Real-time Settlement',
    desc: 'Settled instantly at your wholesale discount the moment a top-up arrives — the spread is your profit.',
  },
  {
    icon: Headphones,
    title: 'Technical Support',
    desc: 'The platform keeps the service running and maintains the upstream model ecosystem so you can focus on operations.',
  },
  {
    icon: Boxes,
    title: 'Rich Model Ecosystem',
    desc: 'Aggregates 40+ upstream AI providers; models and channels are maintained centrally by the platform.',
  },
]

const STEPS = [
  {
    icon: FileText,
    title: 'Submit Application',
    desc: 'Fill in your contact details and operating plan to submit an affiliate application.',
  },
  {
    icon: Server,
    title: 'Configure Domain',
    desc: 'Point your domain to the deployment and the platform opens your independent sub-site.',
  },
  {
    icon: Palette,
    title: 'Customize Your Site',
    desc: 'Set up branding, pricing, payment and guides to build your own dedicated AI site.',
  },
  {
    icon: Coins,
    title: 'Start Earning',
    desc: 'Sell card keys and collect top-ups, settled in real time at your wholesale discount.',
  },
]

// NOTE(替换): 下方分成数据为占位示例，请替换为你的真实分成比例 / 结算周期 / 起步门槛。
const REVENUE_TIERS = [
  {
    tier: 'Starter',
    share: '70%',
    settle: 'Real-time settlement',
    note: 'For individual agents just getting started',
  },
  {
    tier: 'Growth',
    share: '78%',
    settle: 'Real-time settlement',
    note: 'For operators with steady traffic',
  },
  {
    tier: 'Partner',
    share: '85%',
    settle: 'Real-time settlement + priority support',
    note: 'For resellers operating at scale',
  },
]

const FAQS = [
  {
    q: 'Do I need a technical background to become an agent?',
    a: 'No self-development is required. You only need a domain — point it to the deployment per the guide, and the platform opens your independent sub-site. Everything else is configured through a visual dashboard.',
  },
  {
    q: 'How is the revenue share settled?',
    a: 'When users top up online or redeem card keys on your sub-site, the funds go to your own payment account; the platform deducts the cost from your wholesale wallet at your discount, and the spread is your profit — settled in real time within the same transaction.',
  },
  {
    q: 'Will my users and data be mixed with other agents?',
    a: 'No. Each sub-site isolates its users, tokens, logs, card keys and top-up records by site — they are invisible to each other, and different sub-sites are fully independent.',
  },
  {
    q: 'Can I customize the brand and pricing?',
    a: 'Yes. Site name, logo, homepage notice, footer, payment configuration and pricing discounts can all be self-configured in the agent dashboard and are shown automatically per your domain.',
  },
  {
    q: 'Who is responsible for the upstream models and stability?',
    a: 'The platform centrally maintains 40+ upstream AI providers, models and channels, and keeps the service stable — you just focus on operations and growth.',
  },
]

// ---------------------------------------------------------------------------
// 通用：区块标题（eyebrow + h2 + 副标题），镜像首页 how-it-works 写法
// ---------------------------------------------------------------------------

function SectionHeading({
  eyebrow,
  title,
  subtitle,
}: {
  eyebrow: string
  title: string
  subtitle?: string
}) {
  const { t } = useTranslation()
  return (
    <AnimateInView className='mb-14 text-center md:mb-20'>
      <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
        {t(eyebrow)}
      </p>
      <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
        {t(title)}
      </h2>
      {subtitle && (
        <p className='text-muted-foreground/80 mx-auto mt-4 max-w-2xl text-sm leading-relaxed md:text-base'>
          {t(subtitle)}
        </p>
      )}
    </AnimateInView>
  )
}

// ---------------------------------------------------------------------------
// 1. Hero
// ---------------------------------------------------------------------------

function HeroSection() {
  const { t } = useTranslation()
  return (
    <section className='relative z-10 overflow-hidden px-6 pt-24 pb-16 md:pt-32 md:pb-24 lg:pt-36 lg:pb-28'>
      {/* 径向渐变背景（镜像首页 Hero） */}
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 -z-10 opacity-25 dark:opacity-[0.12]'
        style={{
          background: [
            'radial-gradient(ellipse 60% 50% at 20% 20%, oklch(0.72 0.18 250 / 80%) 0%, transparent 70%)',
            'radial-gradient(ellipse 50% 40% at 80% 15%, oklch(0.65 0.15 200 / 60%) 0%, transparent 70%)',
            'radial-gradient(ellipse 40% 35% at 40% 80%, oklch(0.70 0.12 280 / 40%) 0%, transparent 70%)',
          ].join(', '),
        }}
      />
      {/* 网格背景 */}
      <div
        aria-hidden
        className='absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_30%,black_20%,transparent_100%)] bg-[size:4rem_4rem] opacity-[0.08]'
      />

      <div className='mx-auto grid max-w-6xl grid-cols-1 items-center gap-12 lg:grid-cols-12 lg:gap-8'>
        {/* 左：文案 */}
        <div className='flex flex-col items-start text-left lg:col-span-6'>
          <div
            className='landing-animate-fade-up mb-5 inline-flex items-center gap-1.5 rounded-full border border-blue-500/20 bg-blue-500/5 px-3 py-1.5 text-[11px] font-medium text-blue-600 opacity-0 shadow-xs dark:border-blue-400/20 dark:bg-blue-400/5 dark:text-blue-400'
            style={{ animationDelay: '0ms' }}
          >
            <span className='relative flex size-1.5'>
              <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75' />
              <span className='relative inline-flex size-1.5 rounded-full bg-blue-500 dark:bg-blue-400' />
            </span>
            <span>{t('Affiliate Program · Now Recruiting')}</span>
          </div>

          <h1
            className='landing-animate-fade-up text-[clamp(2.25rem,4.5vw,3.25rem)] leading-[1.15] font-bold tracking-tight'
            style={{ animationDelay: '60ms' }}
          >
            {t('Become Our Exclusive Agent')}
            <br />
            <span className='bg-gradient-to-r from-blue-400 via-violet-400 to-purple-500 bg-clip-text text-transparent'>
              {t('Start Your Business Journey with Ease')}
            </span>
          </h1>

          <p
            className='landing-animate-fade-up text-muted-foreground/80 mt-5 max-w-xl text-base leading-relaxed opacity-0 md:text-[15px]'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'With just a domain, you can own an AI site under your own brand. Customize pricing and homepage, and earn a share from user top-ups and card-key sales — the platform handles the tech and models, you focus on running it.'
            )}
          </p>

          {/* 三条卖点 */}
          <ul
            className='landing-animate-fade-up mt-7 flex w-full max-w-xl flex-col gap-3 opacity-0'
            style={{ animationDelay: '180ms' }}
          >
            {HERO_SELLING_POINTS.map(({ icon: Icon, text }) => (
              <li key={text} className='flex items-center gap-3'>
                <span className='border-border/50 bg-muted/30 text-foreground flex size-8 shrink-0 items-center justify-center rounded-lg border'>
                  <Icon className='size-4' strokeWidth={1.75} />
                </span>
                <span className='text-foreground/85 text-sm md:text-[15px]'>
                  {t(text)}
                </span>
              </li>
            ))}
          </ul>

          {/* 按钮 */}
          <div
            className='landing-animate-fade-up mt-9 flex flex-wrap items-center gap-3 opacity-0'
            style={{ animationDelay: '240ms' }}
          >
            <Button
              className='group h-11 rounded-lg px-5 text-sm font-medium'
              render={<Link to='/contact' />}
            >
              {t('Apply Now')}
              <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
            </Button>
            <Button
              variant='outline'
              className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
              render={<a href='#affiliate-advantages' />}
            >
              {t('Learn More')}
            </Button>
          </div>
        </div>

        {/* 右：主视觉（aff.png，位于 web/default/public/aff.png → 访问路径 /aff.png） */}
        <div
          className='landing-animate-fade-up flex w-full justify-center opacity-0 lg:col-span-6'
          style={{ animationDelay: '320ms' }}
        >
          <div className='relative w-full max-w-lg'>
            {/* 主视觉后方的柔光，呼应项目径向渐变风格 */}
            <div
              aria-hidden
              className='pointer-events-none absolute inset-0 -z-0 opacity-50 dark:opacity-30'
              style={{
                background:
                  'radial-gradient(ellipse 70% 60% at 50% 45%, oklch(0.72 0.16 255 / 28%) 0%, transparent 70%)',
              }}
            />
            <img
              src='/aff.png'
              alt={t('Become Our Exclusive Agent')}
              className='relative z-10 w-full object-contain'
              loading='eager'
            />
          </div>
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 2. 适合人群
// ---------------------------------------------------------------------------

function AudienceSection() {
  const { t } = useTranslation()
  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow='Target Users'
          title='Who It Is For'
          subtitle='Flexible income opportunities for different groups'
        />
        <div className='grid gap-6 sm:grid-cols-2 lg:grid-cols-4'>
          {AUDIENCES.map(({ icon: Icon, name, desc }, i) => (
            <AnimateInView
              key={name}
              delay={i * 100}
              animation='fade-up'
              className='border-border/50 bg-card hover:border-border hover:bg-muted/30 group flex flex-col rounded-2xl border p-6 transition-all duration-300 hover:-translate-y-1'
            >
              <span className='border-border/50 bg-muted/30 text-foreground mb-5 flex size-12 items-center justify-center rounded-xl border transition-colors'>
                <Icon className='size-5' strokeWidth={1.75} />
              </span>
              <h3 className='mb-2 text-base font-semibold'>{t(name)}</h3>
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {t(desc)}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 3. 加盟优势
// ---------------------------------------------------------------------------

function AdvantagesSection() {
  const { t } = useTranslation()
  return (
    <section
      id='affiliate-advantages'
      tabIndex={-1}
      className='border-border/40 relative z-10 scroll-mt-20 border-t px-6 py-24 focus:outline-none md:py-32'
    >
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow='Why Choose Us'
          title='Affiliate Advantages'
          subtitle='The platform handles the tech and model ecosystem — you focus on brand operations and growth'
        />
        <div className='grid gap-6 sm:grid-cols-2 lg:grid-cols-3'>
          {ADVANTAGES.map(({ icon: Icon, title, desc }, i) => (
            <AnimateInView
              key={title}
              delay={(i % 3) * 120}
              animation='fade-up'
              className='border-border/50 bg-card hover:border-border hover:bg-muted/30 flex flex-col rounded-2xl border p-6 transition-all duration-300 hover:-translate-y-1'
            >
              <span className='border-border/50 bg-muted/30 text-foreground mb-5 flex size-12 items-center justify-center rounded-xl border'>
                <Icon className='size-5' strokeWidth={1.75} />
              </span>
              <h3 className='mb-2 text-base font-semibold'>{t(title)}</h3>
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {t(desc)}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 4. 加盟流程（镜像 HowItWorks）
// ---------------------------------------------------------------------------

function StepsSection() {
  const { t } = useTranslation()
  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading eyebrow='How To Start' title='Start in Four Steps' />
        <div className='grid gap-10 sm:grid-cols-2 md:grid-cols-4 md:gap-8'>
          {STEPS.map(({ icon: Icon, title, desc }, i) => (
            <AnimateInView
              key={title}
              delay={i * 150}
              animation='fade-up'
              className='relative flex flex-col items-center text-center'
            >
              <div className='relative mb-6'>
                <div className='text-muted-foreground border-border/50 bg-muted/30 flex size-16 items-center justify-center rounded-2xl border'>
                  <Icon className='size-6' strokeWidth={1.5} />
                </div>
                <div className='bg-foreground text-background absolute -top-2 -right-2 flex size-6 items-center justify-center rounded-full text-xs font-bold'>
                  {i + 1}
                </div>
              </div>
              <h3 className='mb-2 text-base font-semibold'>{t(title)}</h3>
              <p className='text-muted-foreground max-w-[240px] text-sm leading-relaxed'>
                {t(desc)}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 5. 收益 / 分成说明
// ---------------------------------------------------------------------------

function RevenueSection() {
  const { t } = useTranslation()
  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-5xl'>
        <SectionHeading
          eyebrow='Revenue Share'
          title='Revenue & Settlement'
          subtitle='The plans below are examples; final share rates and thresholds are subject to the actual agreement'
        />
        <div className='grid gap-6 md:grid-cols-3 md:gap-8'>
          {REVENUE_TIERS.map(({ tier, share, settle, note }, i) => {
            const featured = i === 1
            return (
              <AnimateInView
                key={tier}
                delay={i * 120}
                animation='fade-up'
                className={
                  featured
                    ? 'border-foreground/70 bg-card relative flex flex-col rounded-2xl border-2 p-7 shadow-sm'
                    : 'border-border/50 bg-card flex flex-col rounded-2xl border p-7'
                }
              >
                {featured && (
                  <span className='bg-foreground text-background absolute -top-3 left-1/2 -translate-x-1/2 rounded-full px-3 py-1 text-[11px] font-semibold'>
                    {t('Recommended')}
                  </span>
                )}
                <span className='text-muted-foreground text-xs font-medium tracking-widest uppercase'>
                  {t(tier)}
                </span>
                <div className='mt-3 flex items-end gap-1'>
                  <span className='text-4xl font-bold tracking-tight'>
                    {share}
                  </span>
                  <span className='text-muted-foreground mb-1 text-sm'>
                    {t('share')}
                  </span>
                </div>
                <p className='text-muted-foreground mt-2 text-sm'>{t(settle)}</p>
                <div className='border-border/50 mt-5 border-t pt-4'>
                  <p className='text-muted-foreground flex items-start gap-2 text-sm leading-relaxed'>
                    <CheckCircle2 className='text-foreground/70 mt-0.5 size-4 shrink-0' />
                    {t(note)}
                  </p>
                </div>
              </AnimateInView>
            )
          })}
        </div>
        {/* NOTE(替换): 上方分成比例 / 结算周期 / 门槛为占位示例，请替换为真实数据。 */}
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 6. FAQ
// ---------------------------------------------------------------------------

function FaqSection() {
  const { t } = useTranslation()
  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-3xl'>
        <SectionHeading eyebrow='FAQ' title='Frequently Asked Questions' />
        <AnimateInView>
          <Accordion className='border-border/60 bg-card rounded-2xl border px-5'>
            {FAQS.map((f, i) => (
              <AccordionItem key={f.q} value={`faq-${i}`}>
                <AccordionTrigger className='py-4 text-[15px]'>
                  {t(f.q)}
                </AccordionTrigger>
                <AccordionContent className='text-muted-foreground pb-4 text-sm leading-relaxed'>
                  {t(f.a)}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </AnimateInView>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 7. CTA 横幅
// ---------------------------------------------------------------------------

function CtaSection() {
  const { t } = useTranslation()
  return (
    <section
      id='affiliate-apply'
      tabIndex={-1}
      className='border-border/40 relative z-10 scroll-mt-20 border-t px-6 py-24 focus:outline-none md:py-32'
    >
      <div className='mx-auto max-w-5xl'>
        <AnimateInView
          animation='scale-in'
          className='border-border/60 bg-card relative overflow-hidden rounded-3xl border px-6 py-16 text-center shadow-sm md:px-12'
        >
          <div
            aria-hidden
            className='pointer-events-none absolute inset-0 -z-10 opacity-40 dark:opacity-20'
            style={{
              background:
                'radial-gradient(ellipse 60% 80% at 50% 0%, oklch(0.72 0.16 255 / 30%) 0%, transparent 70%)',
            }}
          />
          <h2 className='mx-auto max-w-2xl text-2xl font-bold tracking-tight md:text-3xl'>
            {t('Ready to start your journey as an agent?')}
          </h2>
          <p className='text-muted-foreground/80 mx-auto mt-4 max-w-xl text-sm leading-relaxed md:text-base'>
            {t(
              'One domain, one AI site under your own brand — start earning your share today.'
            )}
          </p>
          <div className='mt-8 flex justify-center'>
            <Button
              className='group h-11 rounded-lg px-6 text-sm font-medium'
              render={<Link to='/contact' />}
            >
              {t('Apply Now')}
              <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
            </Button>
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 页面入口
// ---------------------------------------------------------------------------

export function Affiliate() {
  return (
    <PublicLayout showMainContainer={false}>
      <main className='overflow-x-hidden'>
        <HeroSection />
        <AudienceSection />
        <AdvantagesSection />
        <StepsSection />
        <RevenueSection />
        <FaqSection />
        <CtaSection />
        <Footer />
      </main>
    </PublicLayout>
  )
}

export default Affiliate

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
  Gift,
  Banknote,
  Info,
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
    text: 'User payments land directly in your own account — set your own retail price, the margin is yours',
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
    desc: 'Turn your fan traffic into your own paying users — their payments land directly in your account.',
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
    title: 'Payments Go Straight to You',
    desc: "Users pay into your own payment account and it arrives instantly — no platform split and nothing to withdraw.",
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
    desc: 'Sell card keys and collect payments — the money lands in your own account, and you stock at your wholesale discount.',
  },
]

// 钱包模式：平台给代理「进货折扣」，代理按折扣从进货钱包扣货款拿货，自定义售价，
// 差价全归代理（用户付款直接进代理自己的收款账户，无分成 / 抽成 / 提现）。
// NOTE(替换): 下方折扣档位为占位示例，请替换为你的真实进货折扣 / 适用门槛。
const WHOLESALE_TIERS = [
  {
    tier: 'Starter',
    rate: '80%',
    note: 'For individual agents just getting started',
  },
  {
    tier: 'Growth',
    rate: '70%',
    note: 'For operators with steady traffic',
  },
  {
    tier: 'Partner',
    rate: '60%',
    note: 'For resellers operating at scale',
  },
]

// ---------------------------------------------------------------------------
// 邀请返佣计划（与上方「品牌代理 / 进货折扣」彼此独立的另一条变现路径）。
// 两张卡片：① 普通邀请返佣（进平台余额）② 专业推广者（线下现金结算）。
// ---------------------------------------------------------------------------
const REFERRAL_TIERS = [
  {
    icon: Gift,
    title: 'Referral Rewards (Open to Everyone)',
    desc: 'Once you sign up you get a personal invite link. After an invited user makes their first successful top-up, you earn a set percentage commission on every top-up they make from then on — credited to your platform balance and transferable anytime.',
    points: [
      'No barrier — every account gets a personal invite link',
      'Recurring — earn on every top-up they make, not just the first',
      'Credited to your balance — transfer and spend it anytime',
    ],
  },
  {
    icon: Banknote,
    title: 'Professional Promoter (Cash Settlement)',
    desc: 'For partners with steady traffic and real promotional reach. Once approved as a Professional Promoter, your commission is settled to you in cash off-platform on a regular cycle instead of being credited to your platform balance — with the cash owed and cash paid clearly shown on your referral page.',
    points: [
      'Paid in cash — settled off-platform, no need to spend on the platform',
      'Transparent ledger — cash owed and cash paid visible in real time',
      'By application — contact us to upgrade to Professional Promoter',
    ],
  },
]

// 区别说明：明确把「邀请返佣」和「品牌代理」划开，避免访客混淆两种模式。
const REFERRAL_VS_AGENT = [
  {
    label: 'Brand Agent',
    desc: 'open an independent AI site on your own domain, stock at a wholesale discount and set your own retail price for the margin — payments land in your own account.',
  },
  {
    label: 'Referral Rewards',
    desc: 'no site to build — you simply bring users to this platform and earn commission on their top-ups.',
  },
]

const FAQS = [
  {
    q: 'Do I need a technical background to become an agent?',
    a: 'No self-development is required. You only need a domain — point it to the deployment per the guide, and the platform opens your independent sub-site. Everything else is configured through a visual dashboard.',
  },
  {
    q: 'How does the money flow — is there any commission or withdrawal?',
    a: 'There is no commission, revenue share or withdrawal. Users pay directly into your own payment account. When you generate redemption codes or a payment is processed, the platform deducts the cost from your wholesale wallet at your discount — the difference between the retail price you set and your wholesale cost is entirely yours.',
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
              'With just a domain, you can own an AI site under your own brand. Stock at a wholesale discount, set your own retail price, and keep the full margin — user payments land directly in your own account, while the platform handles the tech and models.'
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

        {/* 右：主视觉（aff.webp，位于 web/default/public/aff.webp → 访问路径 /aff.webp） */}
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
              src='/aff.webp'
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
// 5. 进货折扣档位（钱包模式：进货价越低、利润越高）
// ---------------------------------------------------------------------------

function RevenueSection() {
  const { t } = useTranslation()
  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-5xl'>
        <SectionHeading
          eyebrow='Wholesale Pricing'
          title='Wholesale Discount Tiers'
          subtitle='The lower your wholesale rate, the higher your margin — the tiers below are examples, final terms are subject to the actual agreement.'
        />
        <div className='grid gap-6 md:grid-cols-3 md:gap-8'>
          {WHOLESALE_TIERS.map(({ tier, rate, note }, i) => {
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
                    {rate}
                  </span>
                  <span className='text-muted-foreground mb-1 text-sm'>
                    {t('of retail')}
                  </span>
                </div>
                <p className='text-muted-foreground mt-2 text-sm'>
                  {t('You stock at this rate; the full margin is yours.')}
                </p>
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
        {/* NOTE(替换): 上方进货折扣 / 门槛为占位示例，请替换为真实数据。 */}
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// 5.5 邀请返佣计划（独立于「品牌代理」的另一条更轻的变现路径）
// ---------------------------------------------------------------------------

function ReferralProgramSection() {
  const { t } = useTranslation()
  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow='Another Way to Earn'
          title='Referral Rewards Program'
          subtitle='No site to build. Share your personal invite link, and earn commission whenever the users you bring in top up on this platform. This is a separate path from the Brand Agent program above — choose either, they do not affect each other.'
        />

        <div className='grid gap-6 md:grid-cols-2 md:gap-8'>
          {REFERRAL_TIERS.map(({ icon: Icon, title, desc, points }, i) => (
            <AnimateInView
              key={title}
              delay={i * 120}
              animation='fade-up'
              className='border-border/50 bg-card flex flex-col rounded-2xl border p-7'
            >
              <span className='border-border/50 bg-muted/30 text-foreground mb-5 flex size-12 items-center justify-center rounded-xl border'>
                <Icon className='size-5' strokeWidth={1.75} />
              </span>
              <h3 className='mb-2 text-base font-semibold'>{t(title)}</h3>
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {t(desc)}
              </p>
              <ul className='border-border/50 mt-5 space-y-2.5 border-t pt-5'>
                {points.map((p) => (
                  <li
                    key={p}
                    className='text-muted-foreground flex items-start gap-2 text-sm leading-relaxed'
                  >
                    <CheckCircle2 className='text-foreground/70 mt-0.5 size-4 shrink-0' />
                    {t(p)}
                  </li>
                ))}
              </ul>
            </AnimateInView>
          ))}
        </div>

        {/* 区别说明：把「邀请返佣」与「品牌代理」划清界限 */}
        <AnimateInView
          animation='fade-up'
          className='border-border/60 bg-muted/30 mt-8 rounded-2xl border p-6 md:p-8'
        >
          <p className='mb-4 flex items-center gap-2 text-sm font-semibold'>
            <Info className='text-muted-foreground size-4' aria-hidden='true' />
            {t('How is this different from the Brand Agent program?')}
          </p>
          <ul className='space-y-3'>
            {REFERRAL_VS_AGENT.map(({ label, desc }) => (
              <li
                key={label}
                className='text-muted-foreground text-sm leading-relaxed'
              >
                <span className='text-foreground font-semibold'>{t(label)}</span>
                <span className='mx-1.5'>·</span>
                {t(desc)}
              </li>
            ))}
          </ul>
          <p className='text-muted-foreground/80 mt-4 text-sm'>
            {t('The two paths are independent and can run in parallel.')}
          </p>
        </AnimateInView>

        {/* 行动入口 */}
        <div className='mt-10 flex flex-wrap justify-center gap-3'>
          <Button
            className='group h-11 rounded-lg px-5 text-sm font-medium'
            render={<Link to='/referral' />}
          >
            {t('Get my referral link')}
            <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
          </Button>
          <Button
            variant='outline'
            className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
            render={<Link to='/contact' />}
          >
            {t('Apply as a Professional Promoter')}
          </Button>
        </div>
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
              'One domain, one AI site under your own brand — set your price and start earning today.'
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
        <ReferralProgramSection />
        <FaqSection />
        <CtaSection />
        <Footer />
      </main>
    </PublicLayout>
  )
}

export default Affiliate

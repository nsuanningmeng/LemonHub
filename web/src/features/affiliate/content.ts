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
  Globe,
  Wallet,
  Palette,
  Code2,
  Megaphone,
  Users2,
  Store,
  Gift,
  Banknote,
} from 'lucide-react'

export const HERO_SELLING_POINTS = [
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

export const AUDIENCES = [
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

export const ADVANTAGES = [
  {
    title: 'One-click Deployment',
    desc: 'A single deployment and one database can host multiple independent branded sub-sites — no development needed.',
  },
  {
    title: 'Independent Domains',
    desc: 'Each sub-site binds its own domain and automatically shows its dedicated brand by domain.',
  },
  {
    title: 'Custom Homepage & Pricing',
    desc: 'Site name, logo, notices, footer and pricing discounts are all self-service configurable.',
  },
  {
    title: 'Payments Go Straight to You',
    desc: 'Users pay into your own payment account and it arrives instantly — no platform split and nothing to withdraw.',
  },
  {
    title: 'Technical Support',
    desc: 'The platform keeps the service running and maintains the upstream model ecosystem so you can focus on operations.',
  },
  {
    title: 'Rich Model Ecosystem',
    desc: 'Aggregates 40+ upstream AI providers; models and channels are maintained centrally by the platform.',
  },
]

export const STEPS = [
  {
    title: 'Submit Application',
    desc: 'Fill in your contact details and operating plan to submit an affiliate application.',
  },
  {
    title: 'Configure Domain',
    desc: 'Point your domain to the deployment and the platform opens your independent sub-site.',
  },
  {
    title: 'Customize Your Site',
    desc: 'Set up branding, pricing, payment and guides to build your own dedicated AI site.',
  },
  {
    title: 'Start Earning',
    desc: 'Sell card keys and collect payments — the money lands in your own account, and you stock at your wholesale discount.',
  },
]

// 钱包模式：平台给代理「进货折扣」，代理按折扣从进货钱包扣货款拿货，自定义售价，
// 差价全归代理（用户付款直接进代理自己的收款账户，无分成 / 抽成 / 提现）。
// NOTE(替换): 下方折扣档位为占位示例，请替换为你的真实进货折扣 / 适用门槛。
export const WHOLESALE_TIERS = [
  {
    tier: 'Starter',
    rate: '80%',
    note: 'For individual agents just getting started',
  },
  {
    tier: 'Growth',
    recommended: true,
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
export const REFERRAL_TIERS = [
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
export const REFERRAL_VS_AGENT = [
  {
    label: 'Brand Agent',
    desc: 'open an independent AI site on your own domain, stock at a wholesale discount and set your own retail price for the margin — payments land in your own account.',
  },
  {
    label: 'Referral Rewards',
    desc: 'no site to build — you simply bring users to this platform and earn commission on their top-ups.',
  },
]

export const FAQS = [
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

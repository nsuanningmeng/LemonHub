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
import { HStack } from '@astryxdesign/core/HStack'
import { VStack } from '@astryxdesign/core/VStack'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpen,
  Check,
  ChevronDown,
  ChevronUp,
  Circle,
  Copy,
  CreditCard,
  FileText,
  KeyRound,
  ListChecks,
  RadioTower,
  ShieldCheck,
  TerminalSquare,
  Timer,
  type LucideIcon,
} from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import {
  CardStaggerContainer,
  CardStaggerItem,
} from '@/components/page-transition'
import { Button, buttonVariants } from '@/components/ui/button'
import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { fetchTokenKey, getApiKeys } from '@/features/keys/api'
import type { ApiKey } from '@/features/keys/types'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { getUserModels } from '@/lib/api'
import { MOTION_TRANSITION } from '@/lib/motion'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  useApiInfo,
  useDashboardContentVisibility,
} from '../../hooks/use-status-data'
import { AnnouncementsPanel } from './announcements-panel'
import { ApiInfoPanel } from './api-info-panel'
import { FAQPanel } from './faq-panel'
import { PerformanceHealthPanel } from './performance-health-panel'
import { SummaryCards } from './summary-cards'
import { UptimePanel } from './uptime-panel'

const SETUP_GUIDE_VISIBILITY_STORAGE_KEY =
  'dashboard_overview_setup_guide_expanded'

type DashboardActionPath =
  | '/keys'
  | '/wallet'
  | '/playground'
  | '/channels'
  | '/usage-logs'
  | '/pricing'

interface StartStep {
  title: string
  description: string
  to: DashboardActionPath
  icon: LucideIcon
  completed: boolean
}

interface QuickAction {
  title: string
  description: string
  to: DashboardActionPath
  icon: LucideIcon
  tone: 'blue' | 'teal' | 'violet'
  adminOnly?: boolean
}

const QUICK_ACTION_SURFACES = {
  blue: 'surface-blue border-tone-blue-border',
  teal: 'surface-teal border-tone-teal-border',
  violet: 'surface-violet border-tone-violet-border',
}

interface RequestExample {
  endpoint: string
  model: string
  keyName: string
  keyId?: number
  displayKey: string
  ready: boolean
}

interface HeroSignal {
  label: string
  value: string
  icon: LucideIcon
  tone: IconBadgeTone
}

function getSavedSetupGuideExpanded(): boolean | null {
  if (typeof window === 'undefined') return null
  const saved = window.localStorage.getItem(SETUP_GUIDE_VISIBILITY_STORAGE_KEY)
  if (saved === 'expanded') return true
  if (saved === 'collapsed') return false
  return null
}

function saveSetupGuideExpanded(expanded: boolean): void {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(
    SETUP_GUIDE_VISIBILITY_STORAGE_KEY,
    expanded ? 'expanded' : 'collapsed'
  )
}

function getCurrentOrigin(): string {
  if (typeof window === 'undefined') return ''
  return window.location.origin
}

function normalizeEndpoint(sourceUrl?: string): string {
  const fallback = `${getCurrentOrigin()}/v1/chat/completions`
  const trimmed = sourceUrl?.trim()
  if (!trimmed) return fallback

  const withoutTrailingSlash = trimmed.replace(/\/+$/, '')
  if (withoutTrailingSlash.endsWith('/v1/chat/completions')) {
    return withoutTrailingSlash
  }
  if (withoutTrailingSlash.endsWith('/v1')) {
    return `${withoutTrailingSlash}/chat/completions`
  }
  return `${withoutTrailingSlash}/v1/chat/completions`
}

function getPreferredKey(keys: ApiKey[]): ApiKey | null {
  return keys.find((item) => item.status === 1) ?? keys[0] ?? null
}

function formatDisplayKey(key?: string): string {
  if (!key) return 'sk-...'
  if (key.length <= 14) return key
  return `${key.slice(0, 7)}...${key.slice(-4)}`
}

function buildCurlCommand(args: {
  endpoint: string
  apiKey: string
  model: string
}): string {
  return [
    `curl ${args.endpoint} \\`,
    '  -H "Content-Type: application/json" \\',
    `  -H "Authorization: Bearer ${args.apiKey}" \\`,
    `  -d '{"model":"${args.model}","messages":[{"role":"user","content":"Say hello in one sentence."}]}'`,
  ].join('\n')
}

function StartStepItem(props: {
  step: StartStep
  index: number
  isLast: boolean
}) {
  const Icon = props.step.icon
  const StatusIcon = props.step.completed ? Check : Circle

  return (
    <li className='relative flex gap-3 pb-2.5 last:pb-0'>
      {!props.isLast && (
        <span
          className='bg-border absolute top-9 bottom-0 left-4 w-px'
          aria-hidden='true'
        />
      )}
      <span
        className={cn(
          'bg-background relative z-10 flex size-8 shrink-0 items-center justify-center rounded-lg border shadow-xs',
          props.step.completed && 'border-success/30 bg-success/10'
        )}
      >
        <StatusIcon
          className={props.step.completed ? 'text-success size-4' : 'size-4'}
          aria-hidden='true'
        />
      </span>

      <Link
        to={props.step.to}
        className='bg-background/70 hover:bg-muted/50 focus-visible:ring-ring flex min-w-0 flex-1 items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-left shadow-xs transition-colors outline-none focus-visible:ring-2'
      >
        <span className='flex min-w-0 items-start gap-2.5'>
          <span className='bg-muted mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg'>
            <Icon className='size-3.5' aria-hidden='true' />
          </span>
          <span className='flex min-w-0 flex-col gap-0.5'>
            <span className='flex items-center gap-2 text-sm font-medium'>
              <span className='text-muted-foreground font-mono text-xs tabular-nums'>
                {props.index + 1}.
              </span>
              <span className='truncate'>{props.step.title}</span>
            </span>
            <span className='text-muted-foreground line-clamp-1 text-xs'>
              {props.step.description}
            </span>
          </span>
        </span>
        <ArrowRight
          className='text-muted-foreground size-4 shrink-0'
          aria-hidden='true'
        />
      </Link>
    </li>
  )
}

function RequestPreview(props: {
  example: RequestExample
  signals: HeroSignal[]
}) {
  const { t } = useTranslation()
  const shouldReduceMotion = useReducedMotion()
  const [isCopying, setIsCopying] = useState(false)
  const { copyToClipboard } = useCopyToClipboard({ notify: false })
  const previewCurl = buildCurlCommand({
    endpoint: props.example.endpoint,
    apiKey: props.example.displayKey,
    model: props.example.model,
  })
  const previewLines = previewCurl.split('\n')
  const handleCopyRequest = async () => {
    if (!props.example.keyId || isCopying) return

    setIsCopying(true)
    try {
      const result = await fetchTokenKey(props.example.keyId)
      const key = result.success && result.data?.key ? result.data.key : ''
      if (!key) {
        toast.error(result.message || t('Failed to copy to clipboard'))
        return
      }

      const realCurl = buildCurlCommand({
        endpoint: props.example.endpoint,
        apiKey: `sk-${key}`,
        model: props.example.model,
      })
      const copied = await copyToClipboard(realCurl)
      if (copied) {
        toast.success(t('Copied to clipboard'))
      } else {
        toast.error(t('Failed to copy to clipboard'))
      }
    } finally {
      setIsCopying(false)
    }
  }

  return (
    <motion.div
      initial={shouldReduceMotion ? false : { opacity: 0, y: 10, scale: 0.98 }}
      animate={shouldReduceMotion ? undefined : { opacity: 1, y: 0, scale: 1 }}
      transition={MOTION_TRANSITION.slow}
      className='bg-card relative min-w-0 overflow-hidden rounded-lg border p-4'
    >
      <div className='flex items-center justify-between gap-3 border-b pb-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <IconBadge tone='info'>
            <TerminalSquare />
          </IconBadge>
          <div className='min-w-0'>
            <div className='truncate text-sm font-medium'>
              {t('First API request')}
            </div>
            <div className='text-muted-foreground truncate text-xs'>
              {props.example.ready
                ? props.example.keyName
                : t('Create an API key to unlock the real request')}
            </div>
          </div>
        </div>
        {props.example.ready ? (
          <Button
            variant='outline'
            size='sm'
            className='h-7 gap-1.5 px-2 text-xs'
            disabled={isCopying}
            onClick={handleCopyRequest}
            aria-label={t('Copy ready-to-run curl')}
          >
            <Copy data-icon='inline-start' />
            {isCopying ? t('Loading') : t('Copy')}
          </Button>
        ) : (
          <Button size='sm' variant='outline' render={<Link to='/keys' />}>
            {t('Create API Key')}
          </Button>
        )}
      </div>

      <div className='brand-inverse bg-brand-ink text-brand-paper my-4 overflow-hidden rounded-lg p-4 font-mono text-xs'>
        <div className='mb-2 flex items-center gap-1.5'>
          <span className='bg-destructive size-2 rounded-full' />
          <span className='bg-warning size-2 rounded-full' />
          <span className='bg-success size-2 rounded-full' />
        </div>
        <div className='flex flex-col gap-1 overflow-hidden'>
          {previewLines.map((line) => (
            <code key={line} className='truncate' title={line}>
              {line}
            </code>
          ))}
        </div>
      </div>

      <div className='grid gap-2'>
        {props.signals.map((signal) => {
          const Icon = signal.icon

          return (
            <div
              key={signal.label}
              className='flex items-center justify-between gap-3 border-t py-2.5'
            >
              <span className='flex min-w-0 items-center gap-2'>
                <IconBadge tone={signal.tone} size='xs'>
                  <Icon />
                </IconBadge>
                <span className='truncate text-xs font-medium'>
                  {signal.label}
                </span>
              </span>
              <span className='text-muted-foreground shrink-0 text-xs'>
                {signal.value}
              </span>
            </div>
          )
        })}
      </div>
    </motion.div>
  )
}

function QuickActionItem(props: { action: QuickAction }) {
  const Icon = props.action.icon

  return (
    <Link
      to={props.action.to}
      aria-label={props.action.title}
      className={cn(
        'group focus-visible:ring-ring flex h-full min-w-0 flex-col gap-5 rounded-2xl border p-5 text-left transition-shadow outline-none hover:shadow-md focus-visible:ring-2',
        QUICK_ACTION_SURFACES[props.action.tone]
      )}
    >
      <HStack gap={3} justify='between' align='center' className='text-inherit'>
        <Icon className='size-6' aria-hidden='true' />
        <ArrowRight
          className='size-4 transition-transform motion-safe:group-hover:translate-x-1'
          aria-hidden='true'
        />
      </HStack>
      <VStack gap={1.5} className='min-w-0 text-inherit'>
        <span className='text-sm font-semibold'>{props.action.title}</span>
        <span className='line-clamp-2 text-xs leading-relaxed'>
          {props.action.description}
        </span>
      </VStack>
    </Link>
  )
}

export function OverviewDashboard() {
  const { t } = useTranslation()
  const setupGuideId = useId()
  const workspaceActionsId = useId()
  const serviceDeskId = useId()
  const setupGuideToggleRef = useRef<HTMLButtonElement>(null)
  const user = useAuthStore((state) => state.auth.user)
  const { items: apiInfoItems } = useApiInfo()
  const showKeys = useIsSidebarModuleVisible('/keys')
  const showWallet = useIsSidebarModuleVisible('/wallet')
  const showPlayground = useIsSidebarModuleVisible('/playground')
  const showChannels = useIsSidebarModuleVisible('/channels')
  const showUsageLogs = useIsSidebarModuleVisible('/usage-logs')
  const visiblePaths = useMemo<Record<DashboardActionPath, boolean>>(
    () => ({
      '/keys': showKeys,
      '/wallet': showWallet,
      '/playground': showPlayground,
      '/channels': showChannels,
      '/usage-logs': showUsageLogs,
      '/pricing': true,
    }),
    [showKeys, showWallet, showPlayground, showChannels, showUsageLogs]
  )
  const {
    apiInfo: showApiInfoPanel,
    announcements: showAnnouncementsPanel,
    faq: showFAQPanel,
    uptimeKuma: showUptimePanel,
  } = useDashboardContentVisibility()
  const [manualSetupGuideExpanded, setManualSetupGuideExpanded] = useState<
    boolean | null
  >(() => getSavedSetupGuideExpanded())

  const requestCount = Number(user?.request_count ?? 0)
  const remainQuota = Number(user?.quota ?? 0)
  const usedQuota = Number(user?.used_quota ?? 0)
  const isAdmin = Boolean(user?.role && user.role >= ROLE.ADMIN)

  const apiKeysQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'api-keys'],
    queryFn: async () => {
      const result = await getApiKeys({ p: 1, size: 10 })
      return result.success ? (result.data?.items ?? []) : []
    },
    staleTime: 60 * 1000,
  })

  const modelsQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'user-models'],
    queryFn: async () => {
      const result = await getUserModels()
      return result.success ? (result.data ?? []) : []
    },
    staleTime: 5 * 60 * 1000,
  })

  const preferredKey = useMemo(
    () => getPreferredKey(apiKeysQuery.data ?? []),
    [apiKeysQuery.data]
  )

  const allStartSteps = useMemo<StartStep[]>(
    () => [
      {
        title: t('Create API Key'),
        description: t('Create a key for your app or service'),
        to: '/keys',
        icon: KeyRound,
        completed: Boolean(preferredKey),
      },
      {
        title: t('Add credits'),
        description: t('Keep enough balance before production traffic'),
        to: '/wallet',
        icon: CreditCard,
        completed: remainQuota > 0 || usedQuota > 0,
      },
      {
        title: t('Send a request'),
        description: t('Verify routing with Playground or your client'),
        to: '/playground',
        icon: TerminalSquare,
        completed: requestCount > 0,
      },
    ],
    [preferredKey, remainQuota, requestCount, t, usedQuota]
  )

  const startSteps = useMemo(
    () => allStartSteps.filter((step) => visiblePaths[step.to]),
    [allStartSteps, visiblePaths]
  )

  const quickActions = useMemo<QuickAction[]>(
    () => [
      {
        title: t('API Keys'),
        description: t('Create a key for your app or service'),
        to: '/keys',
        icon: KeyRound,
        tone: 'blue',
      },
      {
        title: t('Channels'),
        description: t('Configure upstream providers and routing.'),
        to: '/channels',
        icon: RadioTower,
        tone: 'teal',
        adminOnly: true,
      },
      {
        title: t('Usage Logs'),
        description: t('Inspect requests, errors, and billing details'),
        to: '/usage-logs',
        icon: FileText,
        tone: 'teal',
      },
      {
        title: t('Pricing'),
        description: t('Review model rates before scaling traffic'),
        to: '/pricing',
        icon: BookOpen,
        tone: 'violet',
      },
    ],
    [t]
  )

  const visibleQuickActions = useMemo(
    () =>
      quickActions.filter(
        (action) => visiblePaths[action.to] && (!action.adminOnly || isAdmin)
      ),
    [isAdmin, quickActions, visiblePaths]
  )

  const heroSignals = useMemo<HeroSignal[]>(
    () => [
      {
        label: t('Route active'),
        value: apiInfoItems.length > 0 ? t('Online') : t('Current domain'),
        icon: RadioTower,
        tone: 'info',
      },
      {
        label: t('Auth configured'),
        value: preferredKey ? t('Secured') : t('Needs API key'),
        icon: ShieldCheck,
        tone: 'success',
      },
      {
        label: t('Model selected'),
        value: modelsQuery.data?.[0] ?? t('Loading'),
        icon: Timer,
        tone: 'chart-4',
      },
    ],
    [apiInfoItems.length, modelsQuery.data, preferredKey, t]
  )

  const requestExample = useMemo<RequestExample>(() => {
    const endpoint = normalizeEndpoint(apiInfoItems[0]?.url)
    const model = modelsQuery.data?.[0] ?? 'gpt-4o-mini'
    const keyName = preferredKey?.name ?? t('No API key yet')
    const ready = Boolean(preferredKey?.id && model)

    return {
      endpoint,
      model,
      keyName,
      keyId: preferredKey?.id,
      displayKey: preferredKey
        ? formatDisplayKey(`sk-${preferredKey.key}`)
        : 'sk-...',
      ready,
    }
  }, [apiInfoItems, modelsQuery.data, preferredKey, t])

  const completedStepCount = startSteps.filter((step) => step.completed).length
  const setupComplete = completedStepCount === startSteps.length
  const setupStatusReady = apiKeysQuery.isFetched && Boolean(user)
  const hasSetupSteps = startSteps.length > 0
  const setupGuideExpanded =
    hasSetupSteps &&
    (manualSetupGuideExpanded ?? (setupStatusReady && !setupComplete))
  const showLeftContentPanels =
    isAdmin || showApiInfoPanel || showAnnouncementsPanel || showFAQPanel
  const showContentPanels = showLeftContentPanels || showUptimePanel

  const handleSetupGuideToggle = () => {
    const nextExpanded = !setupGuideExpanded
    setManualSetupGuideExpanded(nextExpanded)
    saveSetupGuideExpanded(nextExpanded)
    if (!nextExpanded && setupComplete) {
      setupGuideToggleRef.current?.focus()
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Workspace overview')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {hasSetupSteps && setupStatusReady && setupComplete && (
          <Button
            ref={setupGuideToggleRef}
            variant='ghost'
            size='sm'
            aria-expanded={setupGuideExpanded}
            aria-controls={setupGuideId}
            onClick={handleSetupGuideToggle}
          >
            <ListChecks data-icon='inline-start' />
            {t('Setup guide')}
          </Button>
        )}
        {showPlayground && (
          <Link to='/playground' className={buttonVariants({ size: 'sm' })}>
            <TerminalSquare data-icon='inline-start' />
            {t('Open playground')}
          </Link>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <VStack gap={8} className='pb-4'>
          <HStack
            gap={4}
            align='end'
            justify='between'
            wrap='wrap'
            className='border-tone-blue-border border-l-4 pl-5'
          >
            <VStack gap={2}>
              <p className='text-tone-blue-strong font-mono text-xs'>
                {t('Your workspace, at a glance')}
              </p>
              <h2 className='text-2xl font-medium tracking-tight sm:text-3xl'>
                {t('Welcome back, {{name}}.', {
                  name: user?.display_name || user?.username || t('there'),
                })}
              </h2>
              <p className='text-muted-foreground max-w-xl text-sm leading-relaxed'>
                {t(
                  'A clear view of your usage. A direct path to your next build.'
                )}
              </p>
            </VStack>
            {showKeys && (
              <Link
                to='/keys'
                className={buttonVariants({ variant: 'outline', size: 'sm' })}
              >
                <KeyRound data-icon='inline-start' />
                {t('Create API Key')}
                <ArrowRight data-icon='inline-end' />
              </Link>
            )}
          </HStack>

          <SummaryCards />

          <section aria-labelledby={workspaceActionsId}>
            <h3 id={workspaceActionsId} className='mb-4 text-sm font-semibold'>
              {t('Workspace shortcuts')}
            </h3>
            <CardStaggerContainer
              className={cn(
                'grid grid-cols-2 gap-3',
                isAdmin ? 'xl:grid-cols-4' : 'xl:grid-cols-3'
              )}
            >
              {visibleQuickActions.map((action) => (
                <CardStaggerItem
                  key={action.title}
                  className='min-w-0 last:odd:col-span-2 xl:last:odd:col-span-1 [&>a]:h-full [&>a]:w-full'
                >
                  <QuickActionItem action={action} />
                </CardStaggerItem>
              ))}
            </CardStaggerContainer>
          </section>

          <section
            id={setupGuideId}
            hidden={!setupGuideExpanded}
            aria-label={t('Setup guide')}
          >
            {setupGuideExpanded && (
              <section
                className={cn(
                  'surface-violet border-tone-violet-border grid gap-6 rounded-2xl border p-5 sm:p-6',
                  showKeys && 'xl:grid-cols-[minmax(0,1fr)_minmax(0,0.8fr)]'
                )}
              >
                <VStack gap={6}>
                  <HStack gap={3} justify='between' align='start' wrap='wrap'>
                    <VStack gap={2} className='min-w-0 flex-1'>
                      <p className='text-tone-violet-strong font-mono text-xs'>
                        {t('Get started')}
                      </p>
                      <h3 className='text-xl font-medium tracking-tight'>
                        {t('Build on your API gateway in minutes')}
                      </h3>
                      <p className='text-muted-foreground text-sm leading-relaxed'>
                        {startSteps.length === 3
                          ? t(
                              'Three small steps. Your first successful request.'
                            )
                          : t('Setup progress: {{completed}}/{{total}}', {
                              completed: completedStepCount,
                              total: startSteps.length,
                            })}
                      </p>
                    </VStack>
                    <Button
                      variant='ghost'
                      size='sm'
                      aria-expanded={setupGuideExpanded}
                      aria-controls={setupGuideId}
                      onClick={handleSetupGuideToggle}
                    >
                      <ChevronUp data-icon='inline-start' />
                      {t('Hide setup guide')}
                    </Button>
                  </HStack>
                  <ol>
                    {startSteps.map((step, index) => (
                      <StartStepItem
                        key={step.title}
                        step={step}
                        index={index}
                        isLast={index === startSteps.length - 1}
                      />
                    ))}
                  </ol>
                </VStack>
                {showKeys && (
                  <RequestPreview
                    example={requestExample}
                    signals={heroSignals}
                  />
                )}
              </section>
            )}
          </section>

          {!setupGuideExpanded && !setupComplete && (
            <HStack
              gap={4}
              wrap='wrap'
              align='center'
              justify='between'
              className='surface-violet border-tone-violet-border rounded-xl border-l-4 px-5 py-4'
            >
              <VStack gap={1}>
                <h3 className='text-sm font-semibold'>{t('Setup guide')}</h3>
                <p className='text-muted-foreground text-xs'>
                  {t('Setup progress: {{completed}}/{{total}}', {
                    completed: completedStepCount,
                    total: startSteps.length,
                  })}
                </p>
              </VStack>
              <Button
                variant='outline'
                size='sm'
                aria-expanded={setupGuideExpanded}
                aria-controls={setupGuideId}
                onClick={handleSetupGuideToggle}
              >
                <ChevronDown data-icon='inline-start' />
                {t('Show setup guide')}
              </Button>
            </HStack>
          )}

          {showContentPanels && (
            <section aria-labelledby={serviceDeskId}>
              <HStack
                gap={4}
                wrap='wrap'
                align='end'
                justify='between'
                className='mb-4 border-t pt-6'
              >
                <VStack gap={1}>
                  <h3 id={serviceDeskId} className='text-sm font-semibold'>
                    {t('Service desk')}
                  </h3>
                  <p className='text-muted-foreground text-xs'>
                    {t('Connections, platform updates, and support.')}
                  </p>
                </VStack>
              </HStack>
              <CardStaggerContainer
                className={cn(
                  'grid grid-cols-1 gap-4',
                  showLeftContentPanels &&
                    showUptimePanel &&
                    'xl:grid-cols-[minmax(0,1fr)_20rem]'
                )}
              >
                {showLeftContentPanels && (
                  <section
                    className={cn(
                      'grid min-w-0 grid-cols-1 gap-4',
                      (showApiInfoPanel ||
                        showAnnouncementsPanel ||
                        showFAQPanel) &&
                        'lg:grid-cols-2'
                    )}
                  >
                    {isAdmin && (
                      <CardStaggerItem className='lg:col-span-2'>
                        <PerformanceHealthPanel />
                      </CardStaggerItem>
                    )}
                    {showApiInfoPanel && (
                      <CardStaggerItem>
                        <ApiInfoPanel />
                      </CardStaggerItem>
                    )}
                    {showAnnouncementsPanel && (
                      <CardStaggerItem>
                        <AnnouncementsPanel />
                      </CardStaggerItem>
                    )}
                    {showFAQPanel && (
                      <CardStaggerItem>
                        <FAQPanel />
                      </CardStaggerItem>
                    )}
                  </section>
                )}
                {showUptimePanel && (
                  <CardStaggerItem>
                    <UptimePanel />
                  </CardStaggerItem>
                )}
              </CardStaggerContainer>
            </section>
          )}
        </VStack>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

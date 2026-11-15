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
  Copy,
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
  completed: boolean
}

interface QuickAction {
  title: string
  to: DashboardActionPath
  icon: LucideIcon
  tone: 'blue' | 'teal' | 'violet'
  adminOnly?: boolean
}

const ACTION_TONES = {
  blue: 'text-tone-blue-strong',
  teal: 'text-tone-teal-strong',
  violet: 'text-tone-violet-strong',
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
  tone: 'blue' | 'teal' | 'violet'
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

function StartStepItem(props: { step: StartStep; index: number }) {
  return (
    <li>
      <Link
        to={props.step.to}
        className='group hover:bg-tone-violet focus-visible:ring-ring flex min-w-0 items-center gap-3 rounded-lg py-3 pr-2 text-left transition-colors outline-none focus-visible:ring-2'
      >
        <span className='text-tone-violet-strong flex size-8 shrink-0 items-center justify-center font-mono text-sm'>
          {props.step.completed ? (
            <Check className='text-success size-4' aria-hidden='true' />
          ) : (
            props.index + 1
          )}
        </span>
        <span className='flex min-w-0 flex-1 flex-col gap-1'>
          <span className='text-sm font-medium'>{props.step.title}</span>
          <span className='text-muted-foreground text-xs leading-relaxed'>
            {props.step.description}
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
    <motion.section
      initial={shouldReduceMotion ? false : { opacity: 0, y: 10, scale: 0.98 }}
      animate={shouldReduceMotion ? undefined : { opacity: 1, y: 0, scale: 1 }}
      transition={MOTION_TRANSITION.slow}
      className='min-w-0'
      aria-label={t('First API request')}
    >
      <HStack gap={3} align='center' justify='between' wrap='wrap'>
        <VStack gap={1} className='min-w-0'>
          <h4 className='text-sm font-medium'>{t('First API request')}</h4>
          <p
            className='text-muted-foreground truncate text-xs'
            title={props.example.keyName}
          >
            {props.example.ready
              ? props.example.keyName
              : t('Create an API key to unlock the real request')}
          </p>
        </VStack>
        {props.example.ready ? (
          <Button
            variant='ghost'
            size='sm'
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
      </HStack>
      <pre
        className='brand-inverse bg-brand-ink text-brand-paper my-4 overflow-x-auto rounded-xl p-5 font-mono text-xs leading-relaxed'
        tabIndex={0}
        aria-label={t('First API request')}
      >
        <code>{previewCurl}</code>
      </pre>
      <dl className='space-y-3'>
        {props.signals.map((signal) => {
          const Icon = signal.icon
          return (
            <HStack key={signal.label} gap={3} align='center' justify='between'>
              <HStack
                as='dt'
                gap={2}
                align='center'
                className='min-w-0 text-xs'
              >
                <Icon
                  className={cn('size-3.5 shrink-0', ACTION_TONES[signal.tone])}
                  aria-hidden='true'
                />
                {signal.label}
              </HStack>
              <dd
                className='text-muted-foreground min-w-0 truncate text-xs'
                title={signal.value}
              >
                {signal.value}
              </dd>
            </HStack>
          )
        })}
      </dl>
    </motion.section>
  )
}

function QuickActionItem(props: { action: QuickAction }) {
  const Icon = props.action.icon
  return (
    <Link
      to={props.action.to}
      aria-label={props.action.title}
      className='group hover:bg-muted/40 focus-visible:ring-ring flex min-w-0 items-center gap-3 rounded-lg px-3 py-4 text-left transition-colors outline-none focus-visible:ring-2'
    >
      <Icon
        className={cn('size-5 shrink-0', ACTION_TONES[props.action.tone])}
        aria-hidden='true'
      />
      <span className='min-w-0 flex-1 text-sm font-medium'>
        {props.action.title}
      </span>
      <ArrowRight
        className='text-muted-foreground size-4 shrink-0'
        aria-hidden='true'
      />
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
        completed: Boolean(preferredKey),
      },
      {
        title: t('Add credits'),
        description: t('Keep enough balance before production traffic'),
        to: '/wallet',
        completed: remainQuota > 0 || usedQuota > 0,
      },
      {
        title: t('Send a request'),
        description: t('Verify routing with Playground or your client'),
        to: '/playground',
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
        to: '/keys',
        icon: KeyRound,
        tone: 'blue',
      },
      {
        title: t('Channels'),
        to: '/channels',
        icon: RadioTower,
        tone: 'teal',
        adminOnly: true,
      },
      {
        title: t('Usage Logs'),
        to: '/usage-logs',
        icon: FileText,
        tone: 'teal',
      },
      {
        title: t('Pricing'),
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
        tone: 'teal',
      },
      {
        label: t('Auth configured'),
        value: preferredKey ? t('Secured') : t('Needs API key'),
        icon: ShieldCheck,
        tone: 'blue',
      },
      {
        label: t('Model selected'),
        value: modelsQuery.data?.[0] ?? t('Loading'),
        icon: Timer,
        tone: 'violet',
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
            className='min-w-0'
          >
            <VStack gap={2} className='min-w-0'>
              <h2 className='text-2xl font-medium tracking-tight wrap-anywhere sm:text-3xl'>
                {t('Welcome back, {{name}}.', {
                  name: user?.display_name || user?.username || t('there'),
                })}
              </h2>
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
                'grid grid-cols-1 gap-x-6 sm:grid-cols-2',
                isAdmin ? 'xl:grid-cols-4' : 'xl:grid-cols-3'
              )}
            >
              {visibleQuickActions.map((action) => (
                <CardStaggerItem key={action.title} className='min-w-0'>
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
                  'grid gap-8 py-2',
                  showKeys && 'xl:grid-cols-[minmax(0,1fr)_minmax(0,0.8fr)]'
                )}
              >
                <VStack gap={6}>
                  <HStack gap={3} justify='between' align='start' wrap='wrap'>
                    <VStack gap={2} className='min-w-0 flex-1'>
                      <h3 className='text-tone-violet-strong text-lg font-medium'>
                        {t('Get started')}
                      </h3>
                      <p className='text-muted-foreground text-sm leading-relaxed'>
                        {t('Setup progress: {{completed}}/{{total}}', {
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
              className='py-3'
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
                className='mb-5'
              >
                <VStack gap={1}>
                  <h3 id={serviceDeskId} className='text-sm font-semibold'>
                    {t('Service desk')}
                  </h3>
                </VStack>
              </HStack>
              <CardStaggerContainer
                className={cn(
                  'grid grid-cols-1 gap-8',
                  showLeftContentPanels &&
                    showUptimePanel &&
                    'xl:grid-cols-[minmax(0,1fr)_20rem]'
                )}
              >
                {showLeftContentPanels && (
                  <section
                    className={cn(
                      'grid min-w-0 grid-cols-1 gap-x-8 gap-y-6',
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

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
import { useState, useMemo, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ChevronLeft,
  Code2,
  ExternalLink,
  Download,
  Eye,
  EyeOff,
  Info,
  Terminal,
  Laptop,
  Puzzle,
  Globe,
  type LucideIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { getUserModels } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog } from '@/components/dialog'
import { CopyButton } from '@/components/copy-button'
import { getCurrentBaseURL } from '../../lib/current-base-url'

// Types based on Shared API Contract
export type ClientArtifact = {
  level: 'l0' | 'l1' | 'l2' | 'l3'
  kind: 'deeplink' | 'snippet' | 'script' | 'steps'
  platform: 'any' | 'windows' | 'darwin' | 'linux'
  contentType: string
  requiresHelperApp: boolean
  label: string
  filename: string
  template: string
}

export type ClientIntegration = {
  id: string
  name: string
  family: 'cli' | 'desktop' | 'editor' | 'web'
  enabled: boolean
  artifacts: ClientArtifact[]
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  tokenId: string | number
}

function detectOS(): 'windows' | 'darwin' | 'linux' {
  const nav = window.navigator as Navigator & {
    userAgentData?: { platform?: string }
  }
  const p = (nav.userAgentData?.platform || nav.platform || '').toLowerCase()
  if (p.includes('win')) return 'windows'
  if (p.includes('mac')) return 'darwin'
  return 'linux'
}

export function ConnectAppDialog({ open, onOpenChange, tokenKey, tokenId }: Props) {
  const { t } = useTranslation()
  const [selectedTool, setSelectedTool] = useState<ClientIntegration | null>(null)
  const [revealKey, setRevealKey] = useState(false)
  const [selectedModel, setSelectedModel] = useState('')
  const [os, setOs] = useState<'windows' | 'darwin' | 'linux'>(detectOS())

  const { data: integrationsData, isLoading: loadingIntegrations } = useQuery({
    queryKey: ['client-integrations'],
    queryFn: async () => {
      const res = await fetch('/api/user/client-integrations')
      const json = await res.json()
      return json.data as ClientIntegration[]
    },
    enabled: open,
  })

  const { data: modelsData } = useQuery({
    queryKey: ['user-models-connect'],
    queryFn: getUserModels,
    enabled: open && !!selectedTool,
  })

  const modelOptions = useMemo(() => {
    return (modelsData?.data ?? []).map((m) => ({ value: m, label: m }))
  }, [modelsData?.data])

  // Effective model: user selection, else first available, else placeholder.
  // Derived (not stored) to avoid setState-in-effect cascading renders.
  const effectiveModel = selectedModel || modelOptions[0]?.value || ''

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setSelectedTool(null)
      setRevealKey(false)
      setSelectedModel('')
    }
  }, [open])

  const renderToolGallery = () => {
    if (loadingIntegrations) return <div className="flex h-40 items-center justify-center"><Code2 className="animate-spin" /></div>
    
    const families = {
      cli: integrationsData?.filter(i => i.family === 'cli') || [],
      desktop: integrationsData?.filter(i => i.family === 'desktop') || [],
      editor: integrationsData?.filter(i => i.family === 'editor') || [],
      web: integrationsData?.filter(i => i.family === 'web') || [],
    }

    const FamilySection = ({ title, tools, icon: Icon }: { title: string, tools: ClientIntegration[], icon: LucideIcon }) => {
      if (tools.length === 0) return null
      return (
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Icon size={16} />
            <span>{t(title)}</span>
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            {tools.map(tool => (
              <button
                key={tool.id}
                onClick={() => setSelectedTool(tool)}
                className="flex flex-col items-center gap-3 rounded-xl border bg-card p-4 transition-all hover:border-primary hover:shadow-md focus:outline-none focus:ring-2 focus:ring-primary"
              >
                <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-primary/5 text-primary">
                  {tool.family === 'cli' ? <Terminal /> : tool.family === 'desktop' ? <Laptop /> : tool.family === 'editor' ? <Puzzle /> : <Globe />}
                </div>
                <span className="text-center text-sm font-semibold">{tool.name}</span>
              </button>
            ))}
          </div>
        </div>
      )
    }

    return (
      <div className="space-y-6">
        <FamilySection title="Desktop Apps" tools={families.desktop} icon={Laptop} />
        <FamilySection title="CLI Tools" tools={families.cli} icon={Terminal} />
        <FamilySection title="Editors & Extensions" tools={families.editor} icon={Puzzle} />
        <FamilySection title="Web Clients" tools={families.web} icon={Globe} />
      </div>
    )
  }

  const realKey = tokenKey.startsWith('sk-') ? tokenKey : `sk-${tokenKey}`
  const MASKED_KEY = 'sk-••••••••••••••••'

  // real=true for FUNCTIONAL outputs (open deeplink / copy / download body) so
  // the produced config always works. real=false (default) is for on-screen
  // display and honors the reveal toggle (privacy).
  const resolveTemplate = (template: string, real = false) => {
    const address = getCurrentBaseURL()
    const key = real || revealKey ? realKey : MASKED_KEY
    const model = effectiveModel || '<your-model>'
    return template
      .replace(/{address}/g, address)
      .replace(/{key}/g, key)
      .replace(/{model}/g, model)
  }

  const handleDownload = async (artifact: ClientArtifact) => {
    try {
      const res = await fetch('/api/user/client-integrations/download', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tokenId,
          tool: selectedTool?.id,
          level: artifact.level,
          platform: os,
          model: effectiveModel
        })
      })
      if (!res.ok) throw new Error('Download failed')
      
      const blob = await res.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = artifact.filename || 'config.sh'
      document.body.appendChild(a)
      a.click()
      window.URL.revokeObjectURL(url)
    } catch {
      toast.error(t('Download failed'))
    }
  }

  const renderToolPanel = () => {
    if (!selectedTool) return null
    const artifacts = selectedTool.artifacts.filter(a => a.platform === 'any' || a.platform === os)
    
    const hasModelPlaceholder = selectedTool.artifacts.some(a => a.template.includes('{model}'))

    return (
      <div className="space-y-6">
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" onClick={() => setSelectedTool(null)}>
            <ChevronLeft />
          </Button>
          <h3 className="text-lg font-semibold">{selectedTool.name}</h3>
        </div>

        <div className="flex flex-col gap-4 rounded-lg bg-amber-50 p-4 text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
          <div className="flex items-start gap-3">
            <Info className="mt-0.5 shrink-0" size={18} />
            <p className="text-sm">
              {t('This panel reveals your API key. Make sure you are in a private environment.')}
            </p>
          </div>
          <Button 
            variant="outline" 
            size="sm" 
            className="w-fit border-amber-200 bg-white/50 hover:bg-white dark:border-amber-900 dark:bg-black/50"
            onClick={() => setRevealKey(!revealKey)}
            aria-pressed={revealKey}
            aria-label={revealKey ? t('Hide Key') : t('Reveal Key')}
          >
            {revealKey ? <EyeOff size={16} className="mr-2" /> : <Eye size={16} className="mr-2" />}
            {revealKey ? t('Hide Key') : t('Reveal Key')}
          </Button>
        </div>

        {hasModelPlaceholder && (
          <div className="space-y-2">
            <Label>{t('Select Model')}</Label>
            <ComboboxInput
              options={modelOptions}
              value={effectiveModel}
              onValueChange={setSelectedModel}
              placeholder={t('Select or enter model name')}
            />
          </div>
        )}

        {selectedTool.family === 'cli' && (
          <Tabs value={os} onValueChange={(v) => setOs(v as 'windows' | 'darwin' | 'linux')}>
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="windows">Windows</TabsTrigger>
              <TabsTrigger value="darwin">macOS</TabsTrigger>
              <TabsTrigger value="linux">Linux</TabsTrigger>
            </TabsList>
          </Tabs>
        )}

        <div className="space-y-4">
          {artifacts.map((artifact, i) => (
            <div key={i} className="space-y-2">
              {artifact.kind === 'deeplink' && (
                <div className="space-y-2">
                  <Button 
                    className="w-full" 
                    onClick={() => window.open(resolveTemplate(artifact.template, true), '_blank')}
                  >
                    <ExternalLink className="mr-2" size={16} />
                    {artifact.label || t('Open App')}
                  </Button>
                  {artifact.requiresHelperApp && (
                    <p className="text-center text-xs text-muted-foreground">
                      {t('Requires the application to be installed.')}
                    </p>
                  )}
                </div>
              )}

              {artifact.kind === 'snippet' && (
                <div className="space-y-2">
                  <Label>{artifact.label || t('Configuration Snippet')}</Label>
                  <div className="relative group">
                    <pre className="overflow-x-auto rounded-lg bg-zinc-950 p-4 text-sm text-zinc-50 font-mono">
                      {resolveTemplate(artifact.template)}
                    </pre>
                    <div className="absolute right-2 top-2 opacity-0 transition-opacity group-hover:opacity-100">
                      <CopyButton value={resolveTemplate(artifact.template, true)} />
                    </div>
                  </div>
                </div>
              )}

              {artifact.kind === 'script' && (
                <Button variant="secondary" className="w-full" onClick={() => handleDownload(artifact)}>
                  <Download className="mr-2" size={16} />
                  {artifact.label || t('Download Config Script')}
                </Button>
              )}

              {artifact.kind === 'steps' && (
                <div className="rounded-lg border bg-muted/50 p-4">
                  <Label className="mb-2 block font-bold">{artifact.label || t('Manual Steps')}</Label>
                  <div className="text-sm prose dark:prose-invert max-w-none">
                    {resolveTemplate(artifact.template).split('\n').map((line, j) => (
                      <p key={j} className="mb-1">{line}</p>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={selectedTool ? t('Configure Integration') : t('Connect to an App')}
      contentClassName="sm:max-w-xl"
      contentHeight="auto"
    >
      {selectedTool ? renderToolPanel() : renderToolGallery()}
    </Dialog>
  )
}

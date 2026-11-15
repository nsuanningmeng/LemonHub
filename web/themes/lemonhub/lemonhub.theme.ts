// The lemonhub theme. `astryx experimental theme editor` wrote this file once and never changes it again.
// The checked settings live in lemonhub.settings.json and the generated lemonhub.settings.ts;
// change them with `astryx experimental theme editor set lemonhub ...`. Add your own changes below: they win over the settings.
import {defineTheme} from '@astryxdesign/core/theme';
import {lemonhubSettings} from './lemonhub.settings';
import {palette as foundation} from './tokens/lemonhub.palette';
import {palette as studio} from './tokens/studio.palette';

export const lemonhubTheme = defineTheme({
  extends: lemonhubSettings,
  name: 'lemonhub',
  // Theme-owned palette roles shared by Astryx and the existing Base UI controls.
  localTokens: {
    '--studio-background': [`color-mix(in oklch, ${foundation.neutral.light[95]} 28%, white)`, foundation.neutral.dark[5]],
    '--studio-foreground': [foundation.neutral.light[15], foundation.neutral.dark[95]],
    '--studio-muted': [foundation.neutral.light[30], foundation.neutral.dark[75]],
    '--studio-card': [foundation.neutral.light[100], foundation.neutral.dark[15]],
    '--studio-border': [foundation.neutral.light[85], foundation.neutral.dark[30]],
    '--studio-primary': [foundation.accent.light[45], foundation.accent.dark[75]],
    '--studio-on-primary': [foundation.neutral.light[100], foundation.accent.dark[10]],
    '--studio-secondary': [foundation.neutral.light[95], foundation.neutral.dark[20]],
    '--studio-accent': [foundation.accent.light[95], foundation.accent.dark[20]],
    '--studio-sidebar': [`color-mix(in oklch, ${foundation.accent.light[95]} 45%, white)`, foundation.neutral.dark[10]],
    '--studio-sidebar-active': [foundation.accent.light[90], foundation.accent.dark[25]],
    '--studio-brand': [foundation.accent.light[45], foundation.accent.dark[35]],
    '--studio-on-brand': [foundation.neutral.light[100], foundation.neutral.dark[100]],
    '--studio-ink': foundation.accent.light[10],
    '--studio-paper': foundation.neutral.light[100],
    '--studio-highlight': studio.blue.light[85],
    '--tone-blue': [studio.blue.light[90], studio.blue.dark[20]],
    '--tone-blue-foreground': [studio.blue.light[20], studio.blue.dark[95]],
    '--tone-blue-strong': [studio.blue.light[35], studio.blue.dark[80]],
    '--tone-blue-border': [studio.blue.light[80], studio.blue.dark[35]],
    '--tone-teal': [studio.teal.light[90], studio.teal.dark[20]],
    '--tone-teal-foreground': [studio.teal.light[20], studio.teal.dark[95]],
    '--tone-teal-strong': [studio.teal.light[35], studio.teal.dark[80]],
    '--tone-teal-border': [studio.teal.light[80], studio.teal.dark[35]],
    '--tone-violet': [studio.violet.light[90], studio.violet.dark[20]],
    '--tone-violet-foreground': [studio.violet.light[20], studio.violet.dark[95]],
    '--tone-violet-strong': [studio.violet.light[35], studio.violet.dark[80]],
    '--tone-violet-border': [studio.violet.light[80], studio.violet.dark[35]],
    '--tone-peach': [studio.peach.light[90], studio.peach.dark[20]],
    '--tone-peach-foreground': [studio.peach.light[20], studio.peach.dark[95]],
    '--tone-peach-strong': [studio.peach.light[35], studio.peach.dark[80]],
    '--tone-peach-border': [studio.peach.light[80], studio.peach.dark[35]],
  },
  // Keep the existing preference system authoritative for both component sets.
  // Its variables include user-selected palettes, density and typography.
  tokens: {
    '--color-background-body': 'var(--background, var(--studio-background))',
    '--color-background-surface': 'var(--card, var(--studio-card))',
    '--color-background-card': 'var(--card, var(--studio-card))',
    '--color-text-primary': 'var(--foreground, var(--studio-foreground))',
    '--color-text-secondary': 'var(--muted-foreground, var(--studio-muted))',
    '--color-accent': 'var(--primary, var(--studio-primary))',
    '--color-on-accent': 'var(--primary-foreground, var(--studio-on-primary))',
    '--color-border': 'var(--border, var(--studio-border))',
    '--font-family-body': 'var(--font-body, "Public Sans Variable", system-ui, sans-serif)',
    '--font-family-heading': 'var(--font-body, "Public Sans Variable", system-ui, sans-serif)',
  },
});

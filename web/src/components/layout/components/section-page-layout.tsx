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
import { Heading } from '@astryxdesign/core/Heading'
import { HStack } from '@astryxdesign/core/HStack'
import { VStack } from '@astryxdesign/core/VStack'
import {
  Children,
  isValidElement,
  useState,
  type ReactElement,
  type ReactNode,
} from 'react'

import { useFixedContentLayout } from '@/hooks/use-fixed-content-layout'
import { cn } from '@/lib/utils'

import { Main } from './main'
import { PageFooterProvider } from './page-footer'

type SlotProps = { children?: ReactNode }

function SectionPageLayoutTitle(_props: SlotProps) {
  return null
}
SectionPageLayoutTitle.displayName = 'SectionPageLayout.Title'

function SectionPageLayoutActions(_props: SlotProps) {
  return null
}
SectionPageLayoutActions.displayName = 'SectionPageLayout.Actions'

function SectionPageLayoutContent(_props: SlotProps) {
  return null
}
SectionPageLayoutContent.displayName = 'SectionPageLayout.Content'

function SectionPageLayoutBreadcrumb(_props: SlotProps) {
  return null
}
SectionPageLayoutBreadcrumb.displayName = 'SectionPageLayout.Breadcrumb'

export type SectionPageLayoutProps = {
  children: ReactNode
  fixedContent?: boolean
}

export function SectionPageLayout(props: SectionPageLayoutProps) {
  const fixedLayout = useFixedContentLayout()
  const [footerContainer, setFooterContainer] = useState<HTMLDivElement | null>(
    null
  )

  let title: ReactNode = null
  let actions: ReactNode = null
  let content: ReactNode = null
  let breadcrumb: ReactNode = null

  Children.forEach(props.children, (node) => {
    if (!isValidElement(node)) return
    const child = node as ReactElement<SlotProps>
    if (child.type === SectionPageLayoutTitle) title = child.props.children
    else if (child.type === SectionPageLayoutActions) {
      actions = child.props.children
    } else if (child.type === SectionPageLayoutContent) {
      content = child.props.children
    } else if (child.type === SectionPageLayoutBreadcrumb) {
      breadcrumb = child.props.children
    }
  })

  return (
    <PageFooterProvider container={footerContainer}>
      <Main className={!fixedLayout ? 'overflow-y-auto' : undefined}>
        <VStack
          gap={3}
          as='header'
          className='shrink-0 px-4 pt-5 pb-4 sm:px-6 sm:pt-7 sm:pb-5 lg:px-8'
        >
          {breadcrumb != null && (
            <HStack className='text-muted-foreground text-xs'>
              {breadcrumb}
            </HStack>
          )}
          <HStack gap={4} wrap='wrap' vAlign='center' hAlign='between'>
            <VStack
              gap={1}
              className='border-primary min-w-0 flex-1 basis-32 border-s-4 ps-3'
            >
              <Heading
                level={1}
                weight='semibold'
                className='text-foreground truncate text-xl tracking-tight sm:text-2xl'
              >
                {title}
              </Heading>
            </VStack>
            {actions != null && (
              <HStack
                gap={2}
                wrap='wrap'
                vAlign='center'
                hAlign='end'
                className='max-w-full min-w-0'
              >
                {actions}
              </HStack>
            )}
          </HStack>
        </VStack>

        <VStack
          className={cn(
            'min-w-0 px-4 pb-4 sm:px-6 sm:pb-6 lg:px-8',
            fixedLayout ? 'min-h-0 flex-1' : 'shrink-0',
            fixedLayout &&
              (props.fixedContent ? 'overflow-hidden' : 'overflow-auto')
          )}
        >
          {content}
        </VStack>

        <div
          ref={setFooterContainer}
          className='bg-background shrink-0 border-t px-4 py-3 empty:hidden sm:px-6 lg:px-8'
        />
      </Main>
    </PageFooterProvider>
  )
}

SectionPageLayout.Title = SectionPageLayoutTitle
SectionPageLayout.Actions = SectionPageLayoutActions
SectionPageLayout.Content = SectionPageLayoutContent
SectionPageLayout.Breadcrumb = SectionPageLayoutBreadcrumb

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
import type { Control } from 'react-hook-form'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { CampaignFormValues } from './lib/campaign-form'

type CampaignSelectFieldProps = {
  control: Control<CampaignFormValues>
  name: 'type' | 'audience'
  label: string
  options: { value: string; label: string }[]
  description?: string
  onValueChange: () => void
}

export function CampaignSelectField(props: CampaignSelectFieldProps) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem className='min-w-0 content-start'>
          <FormLabel>{props.label}</FormLabel>
          <Select
            name={field.name}
            value={field.value}
            items={props.options}
            onValueChange={(value) => {
              if (value === null || value === field.value) return
              field.onChange(value)
              props.onValueChange()
            }}
          >
            <FormControl>
              <SelectTrigger
                ref={field.ref}
                onBlur={field.onBlur}
                className='w-full min-w-0'
              >
                <SelectValue className='min-w-0'>
                  <span className='truncate'>
                    {
                      props.options.find(
                        (option) => option.value === field.value
                      )?.label
                    }
                  </span>
                </SelectValue>
              </SelectTrigger>
            </FormControl>
            <SelectContent
              align='start'
              alignItemWithTrigger={false}
              className='max-w-[calc(100vw-2rem)]'
            >
              <SelectGroup>
                {props.options.map((option) => (
                  <SelectItem
                    key={option.value}
                    value={option.value}
                    className='[&_[data-slot=select-item-text]]:min-w-0 [&_[data-slot=select-item-text]]:shrink [&_[data-slot=select-item-text]]:break-words [&_[data-slot=select-item-text]]:whitespace-normal'
                  >
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          {props.description && (
            <FormDescription className='text-xs'>
              {props.description}
            </FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

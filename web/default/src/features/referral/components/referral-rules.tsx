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
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

interface ReferralRulesProps {
  /** Effective recharge-commission rate (0-100). When > 0 the rule states the actual rate. */
  commissionPercent?: number
}

export function ReferralRules({ commissionPercent }: ReferralRulesProps) {
  const { t } = useTranslation()

  const percent =
    commissionPercent && commissionPercent > 0
      ? parseFloat(commissionPercent.toFixed(2))
      : null

  const rules = [
    t("Rewards are credited only after your invitee's first successful top-up."),
    percent === null
      ? t('After that, you earn a commission on every top-up they make.')
      : t(
          'After that, you earn a {{percent}}% commission on every top-up they make.',
          { percent }
        ),
  ]

  return (
    <Card data-card-hover='false' size='sm'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Info className='text-muted-foreground size-4' aria-hidden='true' />
          {t('Referral Rules')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <ul className='text-muted-foreground space-y-2 text-sm'>
          {rules.map((rule) => (
            <li key={rule} className='flex gap-2'>
              <span
                className='bg-muted-foreground/40 mt-2 size-1.5 shrink-0 rounded-full'
                aria-hidden='true'
              />
              <span>{rule}</span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

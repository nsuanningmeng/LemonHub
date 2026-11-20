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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { AuthLayout } from '../auth-layout'
import { ForgotPasswordForm } from './components/forgot-password-form'

export function ForgotPassword() {
  const { t } = useTranslation()
  return (
    <AuthLayout
      title={t('Reset password')}
      description={t('We will send a reset link to your registered email.')}
      secondaryAction={
        <div className='flex flex-wrap items-center gap-4'>
          <Link
            to='/sign-in'
            className='text-foreground font-medium underline underline-offset-4'
          >
            {t('Back to sign in')}
          </Link>
          <Link to='/sign-up' className='underline underline-offset-4'>
            {t('Create an account')}
          </Link>
        </div>
      }
    >
      <ForgotPasswordForm />
    </AuthLayout>
  )
}

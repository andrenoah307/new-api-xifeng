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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const POLICY_PLACEHOLDER = `{
  "min_account_age_days": 7,
  "default_generate_quota": 5,
  "default_code_max_uses": 1,
  "default_code_valid_days": 30,
  "group_quotas": {
    "default": 5
  },
  "role_quotas": {
    "10": -1
  }
}`

const invitationCodeSchema = z.object({
  InvitationCodeEnabled: z.boolean(),
  InvitationCodeOAuthRequired: z.boolean(),
  InvitationCodeUserGenerateEnabled: z.boolean(),
  // 后端 setting.UpdateInvitationCodePolicy 会再校验一次字段取值；
  // 这里只挡住语法错误，避免开关已保存而策略写入失败的半成功状态。
  InvitationCodePolicy: z.string().refine((value) => {
    if (!value.trim()) return true
    try {
      const parsed: unknown = JSON.parse(value)
      return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)
    } catch {
      return false
    }
  }, 'Please enter a valid JSON object'),
})

type InvitationCodeFormValues = z.infer<typeof invitationCodeSchema>

type InvitationCodeSectionProps = {
  defaultValues: InvitationCodeFormValues
}

export function InvitationCodeSection({
  defaultValues,
}: InvitationCodeSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<InvitationCodeFormValues>({
    resolver: zodResolver(invitationCodeSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (data: InvitationCodeFormValues) => {
    const updates = Object.entries(data).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof InvitationCodeFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Invitation Codes')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='InvitationCodeEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Require invitation code for password registration')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Users registering with a username and password must provide a valid invitation code'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='InvitationCodeOAuthRequired'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Require invitation code for OAuth / WeChat registration')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Applies only when a third-party sign-in creates a new account'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='InvitationCodeUserGenerateEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Allow regular users to generate invitation codes')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Shows the self-service invitation code card on the wallet page'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='InvitationCodePolicy'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Invitation code policy (JSON)')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={12}
                    className='font-mono text-xs'
                    placeholder={POLICY_PLACEHOLDER}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Configure the minimum account age, default generation quota, default max uses and validity, plus per-group and per-role overrides'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

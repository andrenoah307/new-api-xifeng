import { useTranslation } from 'react-i18next'

import { MultiSelect, type Option } from '@/components/multi-select'
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

interface Props {
  id: string
  label: string
  enabled: boolean
  onEnabledChange: (value: boolean) => void
  scope: string
  onScopeChange: (value: string) => void
  groups: string[]
  onGroupsChange: (value: string[]) => void
  options: Option[]
}

export function MonitoringCacheFilter(props: Props) {
  const { t } = useTranslation()
  return (
    <FieldGroup className='gap-3'>
      <Field orientation='horizontal'>
        <FieldLabel htmlFor={props.id}>{props.label}</FieldLabel>
        <Switch
          id={props.id}
          checked={props.enabled}
          onCheckedChange={props.onEnabledChange}
        />
      </Field>
      <Field data-disabled={!props.enabled}>
        <FieldTitle id={`${props.id}-scope-label`}>
          {t('Filter scope')}
        </FieldTitle>
        <ToggleGroup
          aria-labelledby={`${props.id}-scope-label`}
          variant='outline'
          disabled={!props.enabled}
          value={[props.scope]}
          onValueChange={(value) => {
            if (value.length) props.onScopeChange(value[0])
          }}
        >
          <ToggleGroupItem value='all'>{t('All groups')}</ToggleGroupItem>
          <ToggleGroupItem value='selected'>
            {t('Selected groups')}
          </ToggleGroupItem>
        </ToggleGroup>
      </Field>
      {props.scope === 'selected' && (
        <Field data-disabled={!props.enabled}>
          <FieldLabel htmlFor={`${props.id}-groups`}>
            {t('Selected groups')}
          </FieldLabel>
          <MultiSelect
            id={`${props.id}-groups`}
            disabled={!props.enabled}
            options={props.options}
            selected={props.groups}
            onChange={props.onGroupsChange}
          />
        </Field>
      )}
    </FieldGroup>
  )
}

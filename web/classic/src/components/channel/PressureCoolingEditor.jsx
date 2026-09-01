/*
Copyright (C) 2025 QuantumNous

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
import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Avatar,
  Card,
  Col,
  InputNumber,
  Row,
  Select,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { IconBolt } from '@douyinfe/semi-icons';
import {
  cleanPressureCoolingGroups,
  getPressureCoolingGroupOptions,
  isPressureCoolingSaveAllowed,
  normalizePressureCooling,
  serializePressureCooling,
} from './PressureCoolingEditor.utils';

export {
  cleanPressureCoolingGroups,
  getPressureCoolingGroupOptions,
  isPressureCoolingSaveAllowed,
  isPressureCoolingSaveable,
  normalizePressureCooling,
  parsePressureCooling,
  serializePressureCooling,
} from './PressureCoolingEditor.utils';

const { Text } = Typography;

const DEFAULT_VALUE = Object.freeze({
  enabled: null,
  frt_threshold_ms: null,
  trigger_percent: null,
  cooldown_seconds: null,
  observation_window_seconds: null,
  scope: 'channel',
  cooldown_groups: [],
});

const FieldLabel = ({ children }) => (
  <div style={{ marginBottom: 6 }}>
    <Text type='secondary' size='small'>
      {children}
    </Text>
  </div>
);

const PressureCoolingEditor = ({ value, onChange, groups = [] }) => {
  const { t } = useTranslation();

  const v = useMemo(() => {
    const normalized = normalizePressureCooling(value);
    if (!normalized) return DEFAULT_VALUE;
    return {
      ...normalized,
      cooldown_groups: cleanPressureCoolingGroups(
        normalized.cooldown_groups,
        groups,
      ),
    };
  }, [groups, value]);

  const hasOverride =
    v.enabled != null ||
    v.frt_threshold_ms != null ||
    v.trigger_percent != null ||
    v.cooldown_seconds != null ||
    v.observation_window_seconds != null ||
    (value &&
      typeof value === 'object' &&
      (value.scope != null || value.cooldown_groups != null));

  const update = (patch) => {
    if (typeof onChange !== 'function') return;
    const next = { ...v, ...patch };
    const normalized = normalizePressureCooling(next);
    onChange(
      normalized
        ? {
            ...normalized,
            cooldown_groups: cleanPressureCoolingGroups(
              normalized.cooldown_groups,
              groups,
            ),
          }
        : null,
    );
  };

  const handleToggleOverride = (checked) => {
    if (!checked) {
      onChange(null);
    } else {
      onChange({ ...DEFAULT_VALUE, enabled: true });
    }
  };

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center justify-between mb-3'>
        <div className='flex items-center gap-2'>
          <Avatar
            size='small'
            color='orange'
            className='shadow-md'
            style={{ flexShrink: 0 }}
          >
            <IconBolt size={14} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>{t('压力冷却')}</Text>
            <div
              className='text-xs'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t(
                '当渠道首字延迟持续过高时，自动禁用渠道并在冷却后恢复；未开启则使用默认配置',
              )}
            </div>
          </div>
        </div>
        <Switch checked={hasOverride} onChange={handleToggleOverride} />
      </div>

      {hasOverride && (
        <>
          <Row gutter={16} type='flex' style={{ marginBottom: 16 }}>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>{t('Cooling Scope')}</FieldLabel>
              <Select
                value={v.scope}
                optionList={[
                  { value: 'channel', label: t('Entire Channel') },
                  { value: 'groups', label: t('Specific Groups') },
                ]}
                onChange={(scope) =>
                  update({
                    scope,
                    cooldown_groups:
                      scope === 'groups' ? v.cooldown_groups : [],
                  })
                }
                style={{ width: '100%' }}
              />
            </Col>
            {v.scope === 'groups' && (
              <Col xs={24} md={12} style={{ marginBottom: 8 }}>
                <FieldLabel>{t('Cooldown Groups')}</FieldLabel>
                <Select
                  multiple
                  value={v.cooldown_groups}
                  optionList={getPressureCoolingGroupOptions(groups)}
                  placeholder={t('Select groups to cool')}
                  emptyContent={t('No channel groups available')}
                  onChange={(cooldownGroups) =>
                    update({ cooldown_groups: cooldownGroups })
                  }
                  getPopupContainer={() => document.body}
                  style={{ width: '100%' }}
                />
                {!isPressureCoolingSaveAllowed(v) && (
                  <Text type='danger' size='small'>
                    {t(
                      'At least one cooldown group is required when using specific groups',
                    )}
                  </Text>
                )}
              </Col>
            )}
          </Row>

          <Row gutter={16} type='flex' style={{ marginBottom: 16 }}>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>{t('FRT 阈值 (ms)，留空默认 8000')}</FieldLabel>
              <InputNumber
                min={1000}
                max={60000}
                step={1000}
                value={v.frt_threshold_ms}
                placeholder='8000'
                onChange={(n) =>
                  update({
                    frt_threshold_ms: typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>{t('触发百分比 (%)，留空默认 50')}</FieldLabel>
              <InputNumber
                min={1}
                max={100}
                value={v.trigger_percent}
                placeholder='50'
                suffix='%'
                onChange={(n) =>
                  update({
                    trigger_percent: typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
          </Row>

          <Row gutter={16} type='flex'>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>{t('冷却时长 (秒)，留空默认 300')}</FieldLabel>
              <InputNumber
                min={10}
                max={86400}
                step={30}
                value={v.cooldown_seconds}
                placeholder='300'
                onChange={(n) =>
                  update({
                    cooldown_seconds: typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>{t('观察窗口 (秒)，留空默认 60')}</FieldLabel>
              <InputNumber
                min={10}
                max={3600}
                step={10}
                value={v.observation_window_seconds}
                placeholder='60'
                onChange={(n) =>
                  update({
                    observation_window_seconds:
                      typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
          </Row>
        </>
      )}
    </Card>
  );
};

export default PressureCoolingEditor;

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
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { IconBolt } from '@douyinfe/semi-icons';

const { Text } = Typography;

const DEFAULT_VALUE = Object.freeze({
  enabled: null,
  frt_threshold_ms: null,
  trigger_count: null,
  cooldown_seconds: null,
  observation_window_seconds: null,
});

export const normalizePressureCooling = (value) => {
  if (!value || typeof value !== 'object') return null;
  const hasOverride =
    value.enabled != null ||
    value.frt_threshold_ms != null ||
    value.trigger_count != null ||
    value.cooldown_seconds != null ||
    value.observation_window_seconds != null;
  if (!hasOverride) return null;
  return {
    enabled: value.enabled ?? null,
    frt_threshold_ms: value.frt_threshold_ms ?? null,
    trigger_count: value.trigger_count ?? null,
    cooldown_seconds: value.cooldown_seconds ?? null,
    observation_window_seconds: value.observation_window_seconds ?? null,
  };
};

const FieldLabel = ({ children }) => (
  <div style={{ marginBottom: 6 }}>
    <Text type='secondary' size='small'>
      {children}
    </Text>
  </div>
);

const PressureCoolingEditor = ({ value, onChange }) => {
  const { t } = useTranslation();

  const v = useMemo(() => {
    const src = value && typeof value === 'object' ? value : {};
    return { ...DEFAULT_VALUE, ...src };
  }, [value]);

  const hasOverride =
    v.enabled != null ||
    v.frt_threshold_ms != null ||
    v.trigger_count != null ||
    v.cooldown_seconds != null ||
    v.observation_window_seconds != null;

  const update = (patch) => {
    if (typeof onChange !== 'function') return;
    const next = { ...v, ...patch };
    onChange(normalizePressureCooling(next));
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
            <Text className='text-lg font-medium'>
              {t('压力冷却')}
            </Text>
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
        <Switch
          checked={hasOverride}
          onChange={handleToggleOverride}
        />
      </div>

      {hasOverride && (
        <>
          <Row gutter={16} type='flex' style={{ marginBottom: 16 }}>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>
                {t('FRT 阈值 (ms)，留空默认 8000')}
              </FieldLabel>
              <InputNumber
                min={1000}
                max={60000}
                step={1000}
                value={v.frt_threshold_ms}
                placeholder='8000'
                onChange={(n) =>
                  update({
                    frt_threshold_ms:
                      typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>
                {t('触发次数，留空默认 3')}
              </FieldLabel>
              <InputNumber
                min={1}
                max={100}
                value={v.trigger_count}
                placeholder='3'
                onChange={(n) =>
                  update({
                    trigger_count:
                      typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
          </Row>

          <Row gutter={16} type='flex'>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>
                {t('冷却时长 (秒)，留空默认 300')}
              </FieldLabel>
              <InputNumber
                min={10}
                max={86400}
                step={30}
                value={v.cooldown_seconds}
                placeholder='300'
                onChange={(n) =>
                  update({
                    cooldown_seconds:
                      typeof n === 'number' && n > 0 ? n : null,
                  })
                }
                style={{ width: '100%' }}
                innerButtons
              />
            </Col>
            <Col xs={24} md={12} style={{ marginBottom: 8 }}>
              <FieldLabel>
                {t('观察窗口 (秒)，留空默认 60')}
              </FieldLabel>
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

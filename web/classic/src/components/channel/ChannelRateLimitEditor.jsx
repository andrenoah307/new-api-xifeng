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
import React, { useContext, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Avatar,
  Banner,
  Card,
  Col,
  InputNumber,
  Radio,
  RadioGroup,
  Row,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { IconClock } from '@douyinfe/semi-icons';
import { StatusContext } from '../../context/Status';

const { Text } = Typography;

const ON_LIMIT_VALUES = new Set(['skip', 'queue', 'reject']);

const DEFAULT_VALUE = Object.freeze({
  enabled: false,
  rpm: 0,
  concurrency: 0,
  on_limit: 'skip',
  queue_max_wait_ms: 2000,
  queue_depth: 20,
});

export const normalizeChannelRateLimit = (value) => {
  const src = value && typeof value === 'object' ? value : {};
  const rpm = Number.isFinite(Number(src.rpm))
    ? Math.max(0, Math.floor(Number(src.rpm)))
    : 0;
  const concurrency = Number.isFinite(Number(src.concurrency))
    ? Math.max(0, Math.floor(Number(src.concurrency)))
    : 0;
  const queue_max_wait_ms = Number.isFinite(Number(src.queue_max_wait_ms))
    ? Math.max(0, Math.floor(Number(src.queue_max_wait_ms)))
    : DEFAULT_VALUE.queue_max_wait_ms;
  const queue_depth = Number.isFinite(Number(src.queue_depth))
    ? Math.max(0, Math.floor(Number(src.queue_depth)))
    : DEFAULT_VALUE.queue_depth;
  return {
    enabled: !!src.enabled,
    rpm,
    concurrency,
    on_limit: ON_LIMIT_VALUES.has(src.on_limit)
      ? src.on_limit
      : DEFAULT_VALUE.on_limit,
    queue_max_wait_ms,
    queue_depth,
  };
};

const FieldLabel = ({ children }) => (
  <div style={{ marginBottom: 6 }}>
    <Text type='secondary' size='small'>
      {children}
    </Text>
  </div>
);

const ChannelRateLimitEditor = ({ value, onChange }) => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const redisEnabled = statusState?.status?.redis_enabled !== false;

  const v = useMemo(() => normalizeChannelRateLimit(value), [value]);

  const update = (patch) => {
    if (typeof onChange !== 'function') return;
    onChange(normalizeChannelRateLimit({ ...v, ...patch }));
  };

  const disabled = !v.enabled;

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center justify-between mb-3'>
        <div className='flex items-center gap-2'>
          <Avatar
            size='small'
            color='violet'
            className='shadow-md'
            style={{ flexShrink: 0 }}
          >
            <IconClock size={14} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>{t('渠道限流')}</Text>
            <div
              className='text-xs'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t(
                '限制本渠道的每分钟请求数 (RPM) 与并发数，避免被上游风控；满载时可跳过到同分组其他渠道、串行排队或直接拒绝',
              )}
            </div>
          </div>
        </div>
        <Switch
          checked={v.enabled}
          onChange={(checked) => update({ enabled: checked })}
        />
      </div>

      {!redisEnabled && (
        <Banner
          type='warning'
          fullMode={false}
          closeIcon={null}
          style={{ marginBottom: 16 }}
          description={t(
            '当前未启用 Redis：渠道限流需要 Redis 才能在多副本部署中精准生效。单实例部署可继续使用，但多实例时各副本会各自计数。',
          )}
        />
      )}

      <Row gutter={16} type='flex' style={{ marginBottom: 16 }}>
        <Col xs={24} md={12} style={{ marginBottom: 8 }}>
          <FieldLabel>{t('每分钟请求数 RPM (0 = 不限)')}</FieldLabel>
          <InputNumber
            min={0}
            value={v.rpm}
            disabled={disabled}
            onChange={(n) => update({ rpm: typeof n === 'number' ? n : 0 })}
            style={{ width: '100%' }}
            innerButtons
          />
        </Col>
        <Col xs={24} md={12} style={{ marginBottom: 8 }}>
          <FieldLabel>
            {t('并发数 (同时在飞请求, 0 = 不限)')}
          </FieldLabel>
          <InputNumber
            min={0}
            value={v.concurrency}
            disabled={disabled}
            onChange={(n) =>
              update({ concurrency: typeof n === 'number' ? n : 0 })
            }
            style={{ width: '100%' }}
            innerButtons
          />
        </Col>
      </Row>

      <div style={{ marginBottom: v.on_limit === 'queue' ? 16 : 0 }}>
        <FieldLabel>{t('满载策略')}</FieldLabel>
        <RadioGroup
          direction='vertical'
          value={v.on_limit}
          onChange={(e) => update({ on_limit: e.target.value })}
          disabled={disabled}
          style={{ width: '100%' }}
        >
          <Radio
            value='skip'
            extra={t('当前渠道达限时，自动路由到同分组下其他渠道')}
            style={{ marginBottom: 4 }}
          >
            {t('跳过')}
          </Radio>
          <Radio
            value='queue'
            extra={t(
              '在网关侧串行等待，超过最长等待时长或队列深度后，回退为跳过',
            )}
            style={{ marginBottom: 4 }}
          >
            {t('排队等待')}
          </Radio>
          <Radio
            value='reject'
            extra={t('立即返回 429，不再尝试其他渠道')}
          >
            {t('直接拒绝')}
          </Radio>
        </RadioGroup>
      </div>

      {v.on_limit === 'queue' && (
        <Row gutter={16} type='flex'>
          <Col xs={24} md={12} style={{ marginBottom: 8 }}>
            <FieldLabel>{t('最长等待 (毫秒)')}</FieldLabel>
            <InputNumber
              min={0}
              max={60000}
              step={100}
              value={v.queue_max_wait_ms}
              disabled={disabled}
              onChange={(n) =>
                update({ queue_max_wait_ms: typeof n === 'number' ? n : 0 })
              }
              style={{ width: '100%' }}
              innerButtons
            />
          </Col>
          <Col xs={24} md={12} style={{ marginBottom: 8 }}>
            <FieldLabel>
              {t('队列深度 (最多排队请求数)')}
            </FieldLabel>
            <InputNumber
              min={0}
              max={10000}
              value={v.queue_depth}
              disabled={disabled}
              onChange={(n) =>
                update({ queue_depth: typeof n === 'number' ? n : 0 })
              }
              style={{ width: '100%' }}
              innerButtons
            />
          </Col>
        </Row>
      )}
    </Card>
  );
};

export default ChannelRateLimitEditor;

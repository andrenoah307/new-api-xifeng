import React from 'react';
import { Card, Tag, Progress, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import MiniHistoryChart from './MiniHistoryChart';

const { Text } = Typography;

function getRateColor(rate) {
  if (rate >= 95) return 'var(--semi-color-success)';
  if (rate >= 90) return '#84cc16';
  if (rate >= 85) return 'var(--semi-color-warning)';
  if (rate >= 75) return '#f97316';
  return 'var(--semi-color-danger)';
}

function formatFRT(ms) {
  if (ms == null || ms <= 0) return '-';
  return (ms / 1000).toFixed(2) + 's';
}

const GroupStatusCard = ({ group, onClick }) => {
  const { t } = useTranslation();

  const isOnline = group.is_online;
  const availRate = group.availability_rate ?? 0;
  const cacheRate = group.cache_hit_rate ?? 0;
  const showCache = cacheRate >= 3;

  return (
    <Card
      bodyStyle={{ padding: 16 }}
      style={{
        cursor: 'pointer',
        transition: 'box-shadow 0.2s, transform 0.15s',
        borderRadius: 12,
      }}
      className='group-monitoring-card'
      onClick={() => onClick && onClick(group)}
    >
      {/* Row 1: Group name + status badge */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 8,
        }}
      >
        <Text
          strong
          style={{
            fontSize: 15,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            maxWidth: '70%',
          }}
        >
          {group.group_name}
        </Text>
        <Tag
          color={isOnline ? 'green' : 'red'}
          size='small'
          shape='circle'
          type='light'
        >
          {isOnline ? t('在线') : t('离线')}
        </Tag>
      </div>

      {/* Row 2: test model | FRT | group_ratio */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          marginBottom: 10,
          fontSize: 12,
          color: 'var(--semi-color-text-2)',
          flexWrap: 'wrap',
        }}
      >
        {group.last_test_model && (
          <Text
            size='small'
            type='tertiary'
            style={{
              maxWidth: 120,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {group.last_test_model}
          </Text>
        )}
        {group.first_response_time > 0 && (
          <>
            <span style={{ color: 'var(--semi-color-text-3)' }}>|</span>
            <Text size='small' type='tertiary'>
              FRT {formatFRT(group.first_response_time)}
            </Text>
          </>
        )}
        {group.group_ratio != null && (
          <>
            <span style={{ color: 'var(--semi-color-text-3)' }}>|</span>
            <Text size='small' type='tertiary'>
              {group.group_ratio}
              {t('元/刀')}
            </Text>
          </>
        )}
      </div>

      {/* Availability rate progress */}
      <div style={{ marginBottom: 6 }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 2,
          }}
        >
          <Text size='small' type='tertiary'>
            {t('可用率')}
          </Text>
          <Text
            size='small'
            strong
            style={{ color: getRateColor(availRate) }}
          >
            {availRate.toFixed(1)}%
          </Text>
        </div>
        <Progress
          percent={availRate}
          showInfo={false}
          stroke={getRateColor(availRate)}
          size='small'
          style={{ height: 6 }}
        />
      </div>

      {/* Cache hit rate progress */}
      <div style={{ marginBottom: 10 }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 2,
          }}
        >
          <Text size='small' type='tertiary'>
            {t('缓存命中率')}
          </Text>
          <Text
            size='small'
            strong
            style={{
              color: showCache
                ? getRateColor(cacheRate)
                : 'var(--semi-color-text-2)',
            }}
          >
            {showCache ? `${cacheRate.toFixed(1)}%` : t('尚未获取')}
          </Text>
        </div>
        <Progress
          percent={showCache ? cacheRate : 0}
          showInfo={false}
          stroke={showCache ? getRateColor(cacheRate) : 'var(--semi-color-fill-1)'}
          size='small'
          style={{ height: 6 }}
        />
      </div>

      {/* Mini history chart */}
      {group.history && group.history.length > 0 && (
        <MiniHistoryChart
          history={group.history}
          intervalMinutes={group.aggregation_interval_minutes}
        />
      )}
    </Card>
  );
};

export default GroupStatusCard;

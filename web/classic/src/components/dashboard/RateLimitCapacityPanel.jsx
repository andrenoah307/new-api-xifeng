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

import React, {
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { Banner, Button, Card, Spin, Typography } from '@douyinfe/semi-ui';
import { ChevronDown, ChevronUp, Gauge } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { UserContext } from '../../context/User';

const { Text } = Typography;
const STALE_TIME = 5000;
const RETRY_COOLDOWN = 3000;

function formatCount(value) {
  return Number.isFinite(value) ? value.toLocaleString() : '0';
}

function formatPercent(value) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return null;
  }
  const percent = value * 100;
  if (percent >= 100) {
    return `${Math.ceil(percent * 10) / 10}%`;
  }
  return `${percent.toFixed(1)}%`;
}

function formatObservedAt(value) {
  if (!value) return '--';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '--' : date.toLocaleString();
}

function metricAvailable(metric) {
  return Boolean(
    metric &&
    metric.available &&
    (metric.unlimited ||
      (typeof metric.current === 'number' && Number.isFinite(metric.current))),
  );
}

function CapacityBar({ metric }) {
  if (
    !metric ||
    metric.unlimited ||
    !metricAvailable(metric) ||
    metric.utilization === null ||
    metric.utilization === undefined
  ) {
    return null;
  }

  const width = Math.min(100, Math.max(0, metric.utilization * 100));
  const color = metric.over_limit
    ? 'var(--semi-color-danger)'
    : metric.utilization >= 0.8
      ? 'var(--semi-color-warning)'
      : 'var(--semi-color-primary)';

  return (
    <div
      style={{
        height: 6,
        width: '100%',
        borderRadius: 999,
        background: 'var(--semi-color-fill-2)',
        overflow: 'hidden',
      }}
      aria-hidden='true'
    >
      <div
        style={{
          width: `${width}%`,
          height: '100%',
          borderRadius: 999,
          background: color,
        }}
      />
    </div>
  );
}

function CapacityRow({ label, metric, model, group, t }) {
  const available = metricAvailable(metric);
  const percent = formatPercent(metric?.utilization);
  let value = t('暂时不可用');
  if (available && metric?.unlimited) {
    value = t('无限制');
  } else if (available) {
    value = `${formatCount(metric.current)} / ${formatCount(metric.limit)}`;
  }

  const valueColor = metric?.over_limit
    ? 'var(--semi-color-danger)'
    : available && metric?.utilization >= 0.8
      ? 'var(--semi-color-warning)'
      : undefined;

  return (
    <div className='border-b border-gray-200 px-4 py-3 last:border-b-0 sm:px-5'>
      <div className='flex min-w-0 items-start justify-between gap-3'>
        <div className='min-w-0'>
          <Text strong ellipsis={{ showTooltip: true }}>
            {label}
          </Text>
          {(model || group) && (
            <div className='mt-1 truncate text-xs text-gray-500'>
              {model}
              {group ? ` · ${group}` : ''}
            </div>
          )}
        </div>
        <div
          className='shrink-0 text-right font-mono text-sm font-semibold tabular-nums'
          style={{ color: valueColor }}
        >
          <div>{value}</div>
          {available && percent && (
            <div className='mt-1 text-xs font-normal text-gray-500'>
              {percent}
            </div>
          )}
        </div>
      </div>
      <div className='mt-2'>
        <CapacityBar metric={metric} />
      </div>
    </div>
  );
}

function SiteSection({ title, windowLabel, items, t }) {
  return (
    <section aria-label={title}>
      <div className='border-b border-gray-200 bg-gray-50 px-4 py-3 sm:px-5'>
        <Text strong>{title}</Text>
        <div className='mt-1 text-xs text-gray-500'>
          {windowLabel} · {t('全站用户')}
        </div>
      </div>
      {items.map((item) => (
        <CapacityRow
          key={`${item.model}:${item.group || ''}`}
          label={item.model}
          model={item.model}
          group={item.group}
          metric={item}
          t={t}
        />
      ))}
    </section>
  );
}

function PersonalSection({ personal, t }) {
  const windowLabel =
    personal.status === 'disabled'
      ? t('未启用')
      : t('配置周期：{{minutes}} 分钟', {
          minutes: personal.window_minutes,
        });

  return (
    <section aria-label={t('我的请求限额')}>
      <div className='border-b border-gray-200 bg-gray-50 px-4 py-3 sm:px-5'>
        <Text strong>{t('我的请求限额')}</Text>
        <div className='mt-1 text-xs text-gray-500'>
          {windowLabel}
          {personal.group ? ` · ${personal.group}` : ''}
        </div>
      </div>
      {personal.status === 'disabled' && (
        <div className='px-4 py-4 text-sm text-gray-500 sm:px-5'>
          {t('未启用')}
        </div>
      )}
      {personal.status === 'unconfigured' && (
        <div className='px-4 py-4 text-sm text-gray-500 sm:px-5'>
          {t('未配置')}
        </div>
      )}
      {personal.status !== 'disabled' && personal.status !== 'unconfigured' && (
        <>
          {personal.total && (
            <CapacityRow label={t('总请求数')} metric={personal.total} t={t} />
          )}
          {personal.success && (
            <CapacityRow
              label={t('成功请求数')}
              metric={personal.success}
              t={t}
            />
          )}
          {!personal.total && !personal.success && (
            <div className='px-4 py-4 text-sm text-gray-500 sm:px-5'>
              {t('暂时不可用')}
            </div>
          )}
        </>
      )}
    </section>
  );
}

function CapacityMetadata({ data, t }) {
  const instanceOnly =
    data.instance_only ||
    data.backend_scope === 'instance' ||
    Boolean(data.personal?.instance_only);
  return (
    <div className='flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-xs text-gray-500 sm:px-5'>
      <span>
        {t('快照：{{time}}', { time: formatObservedAt(data.observed_at) })}
      </span>
      <span>
        {t('配置版本：{{model}} / {{group}}', {
          model: data.model_name_rpm_version,
          group: data.group_rate_limit_version,
        })}
      </span>
      <span>{t('快照最多可能滞后 5 秒')}</span>
      {instanceOnly && (
        <span className='text-amber-600'>{t('仅当前实例')}</span>
      )}
    </div>
  );
}

function CapacityExpandButton({
  expanded,
  total,
  loading,
  onIntent,
  onToggle,
  t,
}) {
  let icon = <ChevronDown size={14} />;
  if (expanded) {
    icon = <ChevronUp size={14} />;
  } else if (loading) {
    icon = <Spin size='small' spinning />;
  }
  return (
    <Button
      theme='borderless'
      size='small'
      icon={icon}
      aria-expanded={expanded}
      aria-controls='classic-rate-limit-capacity-all'
      onMouseEnter={onIntent}
      onFocus={onIntent}
      onTouchStart={onIntent}
      onClick={onToggle}
    >
      {expanded ? t('收起更多容量') : t('显示全部 {{total}} 项', { total })}
    </Button>
  );
}

export default function RateLimitCapacityPanel() {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const user = userState?.user;
  const hasUser = Boolean(user);
  const identityKey = `${user?.id || 'anonymous'}:${user?.group || ''}`;
  const [expanded, setExpanded] = useState(false);
  const [topData, setTopData] = useState(null);
  const [allData, setAllData] = useState(null);
  const [topLoading, setTopLoading] = useState(false);
  const [allLoading, setAllLoading] = useState(false);
  const [topError, setTopError] = useState(false);
  const [allError, setAllError] = useState(false);
  const loadedRef = useRef({ top: false, all: false });
  const inFlightRef = useRef({ top: false, all: false });
  const loadedAtRef = useRef({ top: 0, all: 0 });
  const retryAfterRef = useRef({ top: 0, all: 0 });
  const identityRef = useRef(identityKey);
  const initializedIdentityRef = useRef(null);

  const loadCapacity = useCallback(
    async (scope) => {
      if (!hasUser) return;
      const requestIdentity = identityKey;
      const now = Date.now();
      if (
        loadedRef.current[scope] &&
        now - loadedAtRef.current[scope] < STALE_TIME
      ) {
        return;
      }
      if (inFlightRef.current[scope] || now < retryAfterRef.current[scope]) {
        return;
      }

      inFlightRef.current[scope] = true;
      loadedRef.current[scope] = false;
      if (scope === 'top') {
        setTopLoading(true);
        setTopError(false);
      } else {
        setAllLoading(true);
        setAllError(false);
      }

      try {
        const res = await API.get(`/api/rate_limit/capacity?scope=${scope}`, {
          // This is optional dashboard data. Network failures must fail open
          // without the Classic axios interceptor showing a toast.
          skipErrorHandler: true,
        });
        const body = res?.data;
        if (!body?.success || !body.data) {
          throw new Error(body?.message || 'capacity request failed');
        }
        if (identityRef.current !== requestIdentity) return;
        loadedRef.current[scope] = true;
        loadedAtRef.current[scope] = Date.now();
        retryAfterRef.current[scope] = 0;
        if (scope === 'top') {
          setTopData(body.data);
          setTopError(false);
        } else {
          setAllData(body.data);
          setAllError(false);
        }
      } catch {
        if (identityRef.current !== requestIdentity) return;
        retryAfterRef.current[scope] = Date.now() + RETRY_COOLDOWN;
        if (scope === 'top') setTopError(true);
        else setAllError(true);
      } finally {
        if (identityRef.current !== requestIdentity) return;
        inFlightRef.current[scope] = false;
        if (scope === 'top') setTopLoading(false);
        else setAllLoading(false);
      }
    },
    [hasUser, identityKey],
  );

  useEffect(() => {
    if (initializedIdentityRef.current === identityKey) return;
    initializedIdentityRef.current = identityKey;
    identityRef.current = identityKey;
    loadedRef.current = { top: false, all: false };
    inFlightRef.current = { top: false, all: false };
    loadedAtRef.current = { top: 0, all: 0 };
    retryAfterRef.current = { top: 0, all: 0 };
    setTopData(null);
    setAllData(null);
    setTopError(false);
    setAllError(false);
    setExpanded(false);
    if (hasUser) void loadCapacity('top');
  }, [identityKey, hasUser, loadCapacity]);

  const requestAll = useCallback(() => {
    void loadCapacity('all');
  }, [loadCapacity]);

  const toggleAll = () => {
    if (!expanded && !loadedRef.current.all) {
      void loadCapacity('all');
    }
    setExpanded((value) => !value);
  };

  if (topData?.total === 0 || allData?.total === 0) return null;

  const data = (expanded && allData) || topData || allData;
  const site = data?.site;
  const global = site?.global;
  const groups = site?.groups;
  const hasGroups = (site?.groups?.total || 0) > 0;
  const hasHiddenItems = Boolean(
    site &&
    ((global?.total || 0) > (global?.items?.length || 0) ||
      (groups?.total || 0) > (groups?.items?.length || 0)),
  );
  const showExpandControl = hasHiddenItems || expanded;
  const topItems = topData?.site?.groups?.items || site?.groups?.items || [];
  const allItems = allData?.site?.groups?.items || null;

  return (
    <Card
      className='shadow-sm !rounded-2xl'
      title={
        <div className='flex items-center gap-2'>
          <Gauge size={16} />
          {t('RPM 容量')}
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      {topLoading && !topData && (
        <div className='flex min-h-32 items-center justify-center'>
          <Spin spinning />
        </div>
      )}
      {topError && !topData && (
        <Banner
          type='warning'
          title={t('暂时不可用')}
          description={t('容量数据暂时不可用，请稍后重试。')}
        />
      )}
      {data?.degraded && (
        <Banner
          type='warning'
          title={t('容量数据部分不可用')}
          description={t('部分计数器暂时无法读取，不会将不可用数据显示为 0。')}
        />
      )}

      {global?.items?.length > 0 && (
        <SiteSection
          title={t('站点模型 RPM')}
          windowLabel={t('固定 60 秒窗口')}
          items={global.items}
          t={t}
        />
      )}

      {!hasGroups && showExpandControl && (
        <div className='flex items-center justify-between gap-3 border-y border-gray-200 px-4 py-2 sm:px-5'>
          <Text type='tertiary'>{t('站点模型 RPM')}</Text>
          <CapacityExpandButton
            expanded={expanded}
            total={global?.total || 0}
            loading={allLoading}
            onIntent={requestAll}
            onToggle={toggleAll}
            t={t}
          />
        </div>
      )}
      {!hasGroups && (
        <div id='classic-rate-limit-capacity-all' hidden={!expanded}>
          {expanded && allLoading && (
            <div className='flex items-center justify-center gap-2 px-4 py-4 text-xs text-gray-500 sm:px-5'>
              <Spin size='small' spinning />
              {t('加载中...')}
            </div>
          )}
          {expanded && allError && !allData && (
            <div className='px-4 py-3 text-xs text-gray-500 sm:px-5'>
              {t('暂时不可用')}
            </div>
          )}
        </div>
      )}

      {hasGroups && (
        <>
          <div className='flex items-center justify-between gap-3 border-y border-gray-200 px-4 py-2 sm:px-5'>
            <Text type='tertiary'>{t('站点分组 RPM')}</Text>
            {showExpandControl && (
              <CapacityExpandButton
                expanded={expanded}
                total={(global?.total || 0) + (groups?.total || 0)}
                loading={allLoading}
                onIntent={requestAll}
                onToggle={toggleAll}
                t={t}
              />
            )}
          </div>
          {!expanded && (
            <SiteSection
              title={t('站点分组 RPM')}
              windowLabel={t('固定 60 秒窗口')}
              items={topItems}
              t={t}
            />
          )}
          <div id='classic-rate-limit-capacity-all' hidden={!expanded}>
            {expanded && allLoading && (
              <div className='flex items-center justify-center gap-2 px-4 py-4 text-xs text-gray-500 sm:px-5'>
                <Spin size='small' spinning />
                {t('加载中...')}
              </div>
            )}
            {expanded && allError && !allData && (
              <div className='px-4 py-3 text-xs text-gray-500 sm:px-5'>
                {t('暂时不可用')}
              </div>
            )}
            {expanded && (allItems || topItems) && (
              <SiteSection
                title={t('站点分组 RPM')}
                windowLabel={t('固定 60 秒窗口')}
                items={allItems || topItems}
                t={t}
              />
            )}
          </div>
        </>
      )}

      {data?.personal && <PersonalSection personal={data.personal} t={t} />}
      {data && <CapacityMetadata data={data} t={t} />}
    </Card>
  );
}

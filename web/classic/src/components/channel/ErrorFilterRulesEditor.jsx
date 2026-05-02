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
import React, { useMemo, useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Avatar,
  Badge,
  Button,
  Card,
  Checkbox,
  Col,
  Empty,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Spin,
  Tag,
  TagInput,
  Toast,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconPlus,
  IconChevronDown,
  IconChevronUp,
  IconFilter,
  IconRefresh,
  IconHistory,
} from '@douyinfe/semi-icons';
import { API, timestamp2string } from '../../helpers';

const { Text } = Typography;

const ERROR_FILTER_ACTIONS = new Set(['retry', 'rewrite', 'replace']);

const createEmptyRule = () => ({
  status_codes: [],
  message_contains: [],
  error_codes: [],
  action: 'retry',
  rewrite_message: '',
  replace_status_code: 200,
  replace_message: '',
});

const normalizeStringList = (values) =>
  Array.from(
    new Set(
      (Array.isArray(values) ? values : [])
        .map((v) => String(v || '').trim())
        .filter(Boolean),
    ),
  );

const normalizeStatusCodes = (values) =>
  Array.from(
    new Set(
      (Array.isArray(values) ? values : [])
        .map((v) => Number.parseInt(String(v).trim(), 10))
        .filter((v) => Number.isInteger(v) && v >= 100 && v <= 599),
    ),
  );

const normalizeRule = (rule = {}) => {
  const action = ERROR_FILTER_ACTIONS.has(rule.action) ? rule.action : 'retry';
  const replaceStatusCode = Number.parseInt(rule.replace_status_code, 10);
  return {
    status_codes: normalizeStatusCodes(rule.status_codes),
    message_contains: normalizeStringList(rule.message_contains),
    error_codes: normalizeStringList(rule.error_codes),
    action,
    rewrite_message: String(rule.rewrite_message || ''),
    replace_status_code:
      Number.isInteger(replaceStatusCode) && replaceStatusCode >= 100
        ? replaceStatusCode
        : 200,
    replace_message: String(rule.replace_message || ''),
  };
};

// 规则摘要：规则折叠时显示的一行概括
const RuleSummary = ({ rule, t }) => {
  const actionLabel = {
    retry: t('重试'),
    rewrite: t('改写消息'),
    replace: t('替换响应'),
  }[rule.action] || rule.action;

  const actionColor = {
    retry: 'blue',
    rewrite: 'orange',
    replace: 'red',
  }[rule.action] || 'grey';

  const conditions = [];
  if (rule.status_codes.length > 0)
    conditions.push(rule.status_codes.map((c) => `${c}`).join(' / '));
  if (rule.message_contains.length > 0)
    conditions.push(`"${rule.message_contains.slice(0, 2).join('", "')}${rule.message_contains.length > 2 ? '…' : '"'}`);
  if (rule.error_codes.length > 0)
    conditions.push(rule.error_codes.slice(0, 2).join(', ') + (rule.error_codes.length > 2 ? '…' : ''));

  return (
    <div className='flex items-center gap-2 flex-wrap'>
      <Tag color={actionColor} shape='circle' size='small'>{actionLabel}</Tag>
      {conditions.length > 0 ? (
        <Text type='secondary' size='small'>{conditions.join(' · ')}</Text>
      ) : (
        <Text type='tertiary' size='small'>{t('无匹配条件（将匹配所有错误）')}</Text>
      )}
    </div>
  );
};


// 从错误日志中提取可用信息
const parseErrorLog = (log) => {
  let otherData = {};
  try {
    otherData = log.other ? JSON.parse(log.other) : {};
  } catch {
    otherData = {};
  }
  return {
    id: log.id,
    createdAt: log.created_at,
    content: log.content || '',
    modelName: log.model_name || '',
    statusCode: Number.parseInt(otherData.status_code, 10) || null,
    errorCode: otherData.error_code ? String(otherData.error_code) : '',
    errorType: otherData.error_type ? String(otherData.error_type) : '',
  };
};

// 按内容去重，保留最新的一条
const deduplicateErrors = (logs) => {
  const seen = new Map();
  logs.forEach((log) => {
    const key = `${log.statusCode || 0}|${log.errorCode}|${log.content}`;
    if (!seen.has(key)) seen.set(key, log);
  });
  return Array.from(seen.values());
};

// 最近错误记录选择 Modal
const RecentErrorsModal = ({ visible, onClose, channelId, onApply, t }) => {
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState([]);
  const [selectedKeys, setSelectedKeys] = useState(new Set());

  const fetchErrors = async () => {
    if (!channelId) return;
    setLoading(true);
    try {
      const res = await API.get(
        `/api/log/?type=5&channel=${channelId}&p=1&page_size=50`,
      );
      if (res.data?.success) {
        const items = res.data.data?.items || [];
        const parsed = items.map(parseErrorLog);
        setErrors(deduplicateErrors(parsed));
      } else {
        Toast.error(res.data?.message || t('加载错误记录失败'));
      }
    } catch (e) {
      Toast.error(t('加载错误记录失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) {
      setSelectedKeys(new Set());
      fetchErrors();
    }
  }, [visible, channelId]);

  const toggleSelect = (key) => {
    setSelectedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const handleApply = () => {
    const selected = errors.filter((e) => selectedKeys.has(e.id));
    if (selected.length === 0) {
      onClose();
      return;
    }
    const statusCodes = Array.from(
      new Set(selected.map((e) => e.statusCode).filter(Boolean)),
    );
    const errorCodes = Array.from(
      new Set(selected.map((e) => e.errorCode).filter(Boolean)),
    );
    const messages = Array.from(
      new Set(selected.map((e) => e.content).filter(Boolean)),
    );
    onApply({ status_codes: statusCodes, error_codes: errorCodes, messages });
    onClose();
  };

  const selectedCount = selectedKeys.size;

  return (
    <Modal
      centered
      visible={visible}
      onCancel={onClose}
      width={720}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{ maxHeight: 'calc(80vh - 120px)', overflowY: 'auto', overflowX: 'hidden' }}
      title={
        <div className='flex items-center gap-2'>
          <IconHistory />
          <span>{t('从最近错误记录中选择')}</span>
        </div>
      }
      footer={
        <div className='flex items-center justify-between w-full'>
          <Text type='tertiary' size='small'>
            {t('已选 {{n}} 条', { n: selectedCount })}
          </Text>
          <div className='flex gap-2'>
            <Button onClick={onClose}>{t('取消')}</Button>
            <Button
              theme='solid'
              type='primary'
              disabled={selectedCount === 0}
              onClick={handleApply}
            >
              {t('应用选中')}
            </Button>
          </div>
        </div>
      }
    >
      <div className='mb-3 flex items-center justify-between'>
        <Text type='secondary' size='small'>
          {t('展示该渠道最近 50 条错误记录（已按内容去重）')}
        </Text>
        <Button
          icon={<IconRefresh />}
          size='small'
          theme='borderless'
          onClick={fetchErrors}
          loading={loading}
        >
          {t('刷新')}
        </Button>
      </div>

      <Spin spinning={loading}>
        {errors.length === 0 && !loading ? (
          <Empty
            style={{ padding: 30 }}
            description={t('暂无错误记录')}
          />
        ) : (
          <div className='space-y-2'>
            {errors.map((err) => {
              const checked = selectedKeys.has(err.id);
              return (
                <div
                  key={err.id}
                  onClick={() => toggleSelect(err.id)}
                  className='rounded-lg px-3 py-2 cursor-pointer transition-colors'
                  style={{
                    border: `1px solid ${checked ? 'var(--semi-color-primary)' : 'var(--semi-color-border)'}`,
                    backgroundColor: checked
                      ? 'var(--semi-color-primary-light-default)'
                      : 'var(--semi-color-bg-1)',
                  }}
                >
                  <div className='flex items-start gap-2'>
                    <Checkbox
                      checked={checked}
                      onChange={() => toggleSelect(err.id)}
                      onClick={(e) => e.stopPropagation()}
                      style={{ marginTop: 2 }}
                    />
                    <div className='flex-1 min-w-0'>
                      <div className='flex items-center gap-2 flex-wrap mb-1'>
                        {err.statusCode && (
                          <Tag color='blue' shape='circle' size='small'>
                            {err.statusCode}
                          </Tag>
                        )}
                        {err.errorCode && (
                          <Tag color='violet' shape='circle' size='small'>
                            {err.errorCode}
                          </Tag>
                        )}
                        {err.modelName && (
                          <Tag color='grey' shape='circle' size='small'>
                            {err.modelName}
                          </Tag>
                        )}
                        <Text type='tertiary' size='small'>
                          {timestamp2string(err.createdAt)}
                        </Text>
                      </div>
                      <Text
                        size='small'
                        style={{
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                          overflow: 'hidden',
                          wordBreak: 'break-word',
                        }}
                      >
                        {err.content || <Text type='tertiary'>{t('（无消息内容）')}</Text>}
                      </Text>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Spin>
    </Modal>
  );
};

// 单条规则
const RuleItem = ({ rule, index, onUpdate, onRemove, t, actionOptions, channelId }) => {
  const [expanded, setExpanded] = useState(true);
  const [recentModalVisible, setRecentModalVisible] = useState(false);

  const hasCondition =
    rule.status_codes.length > 0 ||
    rule.message_contains.length > 0 ||
    rule.error_codes.length > 0;

  const handleApplyRecentErrors = ({ status_codes, error_codes, messages }) => {
    onUpdate(index, {
      status_codes: Array.from(new Set([...rule.status_codes, ...status_codes])),
      error_codes: Array.from(new Set([...rule.error_codes, ...error_codes])),
      message_contains: Array.from(
        new Set([...rule.message_contains, ...messages]),
      ),
    });
  };

  return (
    <Card
      bodyStyle={{ padding: 0 }}
      style={{ border: '1px solid var(--semi-color-border)', borderRadius: 12 }}
    >
      {/* 规则头部 */}
      <div
        className='flex items-center justify-between px-4 py-3 cursor-pointer select-none'
        style={{ borderBottom: expanded ? '1px solid var(--semi-color-border)' : 'none' }}
        onClick={() => setExpanded((v) => !v)}
      >
        <div className='flex items-center gap-3 min-w-0'>
          <Badge count={index + 1} style={{ backgroundColor: 'var(--semi-color-primary)' }} />
          {!expanded && <RuleSummary rule={rule} t={t} />}
          {expanded && (
            <Text strong size='small'>{t('规则 {{n}}', { n: index + 1 })}</Text>
          )}
        </div>
        <div className='flex items-center gap-1 flex-shrink-0' onClick={(e) => e.stopPropagation()}>
          <Tooltip content={t('删除规则')}>
            <Button
              icon={<IconDelete />}
              type='danger'
              theme='borderless'
              size='small'
              onClick={() => onRemove(index)}
            />
          </Tooltip>
          <Button
            icon={expanded ? <IconChevronUp /> : <IconChevronDown />}
            type='tertiary'
            theme='borderless'
            size='small'
            onClick={() => setExpanded((v) => !v)}
          />
        </div>
      </div>

      {/* 规则内容 */}
      {expanded && (
        <div className='p-4 space-y-4'>
          {/* 匹配条件 */}
          <div>
            <div className='flex items-center justify-between gap-2 mb-3 flex-wrap'>
              <div className='flex items-center gap-1'>
                <Text strong size='small'>{t('匹配条件')}</Text>
                <Text type='tertiary' size='small'>
                  {t('（多类条件 AND，同类条件 OR）')}
                </Text>
              </div>
              {channelId && (
                <Button
                  icon={<IconHistory />}
                  size='small'
                  theme='borderless'
                  type='primary'
                  onClick={() => setRecentModalVisible(true)}
                >
                  {t('从错误记录选择')}
                </Button>
              )}
            </div>
            <div className='space-y-3'>
              <div>
                <Text type='secondary' size='small' className='block mb-1'>{t('HTTP 状态码')}</Text>
                <TagInput
                  value={rule.status_codes.map(String)}
                  placeholder={t('输入状态码后回车，如 429')}
                  onChange={(vals) =>
                    onUpdate(index, { status_codes: normalizeStatusCodes(vals) })
                  }
                  separator={[',', '，', ' ']}
                  style={{ width: '100%' }}
                />
              </div>
              <Row gutter={12}>
                <Col span={12}>
                  <Text type='secondary' size='small' className='block mb-1'>{t('错误码')}</Text>
                  <TagInput
                    value={rule.error_codes}
                    placeholder={t('如 rate_limit_exceeded')}
                    onChange={(vals) =>
                      onUpdate(index, { error_codes: normalizeStringList(vals) })
                    }
                    separator={[',', '，']}
                    style={{ width: '100%' }}
                  />
                </Col>
                <Col span={12}>
                  <Text type='secondary' size='small' className='block mb-1'>{t('消息包含关键词')}</Text>
                  <TagInput
                    value={rule.message_contains}
                    placeholder={t('如 rate limit')}
                    onChange={(vals) =>
                      onUpdate(index, { message_contains: normalizeStringList(vals) })
                    }
                    separator={[',', '，']}
                    style={{ width: '100%' }}
                  />
                </Col>
              </Row>
              {!hasCondition && (
                <div className='text-xs px-2 py-1 rounded' style={{ backgroundColor: 'var(--semi-color-warning-light-default)', color: 'var(--semi-color-warning)' }}>
                  {t('⚠ 未设置任何条件，此规则将匹配所有上游错误')}
                </div>
              )}
            </div>
          </div>

          {/* 执行动作 */}
          <div>
            <Text strong size='small' className='block mb-2'>{t('执行动作')}</Text>
            <Select
              value={rule.action}
              optionList={actionOptions}
              style={{ width: '100%' }}
              onChange={(v) => onUpdate(index, { action: v || 'retry' })}
            />
          </div>

          {/* 动作参数 */}
          {rule.action === 'rewrite' && (
            <div>
              <Text type='secondary' size='small' className='block mb-1'>
                {t('改写消息')}
                <Text type='tertiary' size='small'>{t('（状态码透传给客户端）')}</Text>
              </Text>
              <Input
                value={rule.rewrite_message}
                placeholder={t('输入改写后的错误消息')}
                onChange={(v) => onUpdate(index, { rewrite_message: v })}
                showClear
              />
              {!rule.rewrite_message && (
                <Text type='tertiary' size='small'>{t('留空则消息不变，仅阻止重试和自动禁用')}</Text>
              )}
            </div>
          )}

          {rule.action === 'replace' && (
            <div>
              <Text type='secondary' size='small' className='block mb-1'>{t('替换响应')}</Text>
              <Row gutter={8}>
                <Col span={7}>
                  <InputNumber
                    value={rule.replace_status_code}
                    min={100}
                    max={599}
                    style={{ width: '100%' }}
                    prefix={t('状态码')}
                    onNumberChange={(v) =>
                      onUpdate(index, {
                        replace_status_code:
                          Number.isInteger(v) && v >= 100 ? v : 200,
                      })
                    }
                  />
                </Col>
                <Col span={17}>
                  <Input
                    value={rule.replace_message}
                    placeholder={t('替换后的错误消息')}
                    onChange={(v) => onUpdate(index, { replace_message: v })}
                    showClear
                  />
                </Col>
              </Row>
            </div>
          )}

          {rule.action === 'retry' && (
            <div className='px-3 py-2 rounded-lg text-sm' style={{ backgroundColor: 'var(--semi-color-primary-light-default)', color: 'var(--semi-color-primary)' }}>
              {t('命中后强制切换到下一个渠道重试，忽略原有重试策略')}
            </div>
          )}
        </div>
      )}

      <RecentErrorsModal
        visible={recentModalVisible}
        onClose={() => setRecentModalVisible(false)}
        channelId={channelId}
        onApply={handleApplyRecentErrors}
        t={t}
      />
    </Card>
  );
};

const ErrorFilterRulesEditor = ({ value = [], onChange, channelId }) => {
  const { t } = useTranslation();

  const rules = useMemo(
    () => (Array.isArray(value) ? value : []).map(normalizeRule),
    [value],
  );

  const actionOptions = useMemo(
    () => [
      { label: t('切换渠道重试'), value: 'retry' },
      { label: t('改写消息（透传状态码）'), value: 'rewrite' },
      { label: t('拦截并替换响应'), value: 'replace' },
    ],
    [t],
  );

  const emit = (next) => {
    if (typeof onChange === 'function') onChange(next.map(normalizeRule));
  };

  const updateRule = (index, patch) =>
    emit(rules.map((r, i) => (i === index ? { ...r, ...patch } : r)));

  const removeRule = (index) => emit(rules.filter((_, i) => i !== index));

  const addRule = () => emit([...rules, createEmptyRule()]);

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center justify-between mb-3'>
        <div className='flex items-center gap-2'>
          <Avatar size='small' color='red' className='shadow-md'>
            <IconFilter size={14} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>{t('上游错误过滤')}</Text>
            <div className='text-xs' style={{ color: 'var(--semi-color-text-2)' }}>
              {t('按顺序命中第一条规则，支持重试、改写消息或替换响应')}
            </div>
          </div>
        </div>
        <Button icon={<IconPlus />} theme='light' type='primary' size='small' onClick={addRule}>
          {t('添加规则')}
        </Button>
      </div>

      {rules.length === 0 ? (
        <div
          className='rounded-xl px-4 py-4 text-sm text-center'
          style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
        >
          <Text type='tertiary'>{t('暂无规则')}</Text>
        </div>
      ) : (
        <div className='space-y-2'>
          {rules.map((rule, index) => (
            <RuleItem
              key={index}
              rule={rule}
              index={index}
              onUpdate={updateRule}
              onRemove={removeRule}
              t={t}
              actionOptions={actionOptions}
              channelId={channelId}
            />
          ))}
        </div>
      )}
    </Card>
  );
};

export default ErrorFilterRulesEditor;

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
  Button,
  Card,
  Col,
  Input,
  Row,
  Select,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus, IconShield } from '@douyinfe/semi-icons';

const { Text } = Typography;

// 与后端 dto.RiskControlHeaderRule.Source 保持一致
const RISK_CONTROL_SOURCES = new Set([
  'username',
  'user_id',
  'user_email',
  'user_group',
  'using_group',
  'token_id',
  'request_id',
  'custom',
]);

const createEmptyRule = () => ({
  name: '',
  source: 'username',
  value: '',
});

const normalizeRule = (rule = {}) => {
  const source = RISK_CONTROL_SOURCES.has(rule.source)
    ? rule.source
    : 'username';
  return {
    name: String(rule.name || '').trim(),
    source,
    value: source === 'custom' ? String(rule.value || '') : '',
  };
};

const RuleItem = ({ rule, index, onUpdate, onRemove, t, sourceOptions }) => {
  const isCustom = rule.source === 'custom';
  return (
    <Card
      bodyStyle={{ padding: 12 }}
      style={{ border: '1px solid var(--semi-color-border)', borderRadius: 12 }}
    >
      <Row gutter={8} align='middle'>
        <Col span={isCustom ? 7 : 11}>
          <Text type='secondary' size='small' className='block mb-1'>
            {t('请求头名称')}
          </Text>
          <Input
            value={rule.name}
            placeholder={t('如 X-User-Name')}
            onChange={(v) => onUpdate(index, { name: v })}
            showClear
          />
        </Col>
        <Col span={isCustom ? 7 : 11}>
          <Text type='secondary' size='small' className='block mb-1'>
            {t('数据来源')}
          </Text>
          <Select
            value={rule.source}
            optionList={sourceOptions}
            style={{ width: '100%' }}
            onChange={(v) => onUpdate(index, { source: v || 'username' })}
          />
        </Col>
        {isCustom && (
          <Col span={8}>
            <Text type='secondary' size='small' className='block mb-1'>
              {t('自定义内容')}
            </Text>
            <Input
              value={rule.value}
              placeholder={t('支持 {username} {user_id} 等占位符')}
              onChange={(v) => onUpdate(index, { value: v })}
              showClear
            />
          </Col>
        )}
        <Col span={2} style={{ textAlign: 'right' }}>
          <Tooltip content={t('删除规则')}>
            <Button
              icon={<IconDelete />}
              type='danger'
              theme='borderless'
              size='small'
              onClick={() => onRemove(index)}
            />
          </Tooltip>
        </Col>
      </Row>
      {isCustom && (
        <div className='mt-2'>
          <Text type='tertiary' size='small'>
            {t(
              '占位符：{username} {user_id} {user_email} {user_group} {using_group} {token_id} {request_id}',
            )}
          </Text>
        </div>
      )}
    </Card>
  );
};

const RiskControlHeadersEditor = ({ value = [], onChange }) => {
  const { t } = useTranslation();

  const rules = useMemo(
    () => (Array.isArray(value) ? value : []).map(normalizeRule),
    [value],
  );

  const sourceOptions = useMemo(
    () => [
      { label: t('用户名 (username)'), value: 'username' },
      { label: t('用户 ID (user_id)'), value: 'user_id' },
      { label: t('用户邮箱 (user_email)'), value: 'user_email' },
      { label: t('用户分组 (user_group)'), value: 'user_group' },
      { label: t('使用分组 (using_group)'), value: 'using_group' },
      { label: t('令牌 ID (token_id)'), value: 'token_id' },
      { label: t('请求 ID (request_id)'), value: 'request_id' },
      { label: t('自定义内容'), value: 'custom' },
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
          <Avatar size='small' color='orange' className='shadow-md'>
            <IconShield size={14} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>{t('上游风控识别字段')}</Text>
            <div
              className='text-xs'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t(
                '将 new-api 内部用户/令牌信息作为请求头透传给上游，便于上游做风控、审计或限流',
              )}
            </div>
          </div>
        </div>
        <Button
          icon={<IconPlus />}
          theme='light'
          type='primary'
          size='small'
          onClick={addRule}
        >
          {t('添加字段')}
        </Button>
      </div>

      {rules.length === 0 ? (
        <div
          className='rounded-xl px-4 py-4 text-sm text-center'
          style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
        >
          <Text type='tertiary'>{t('暂无字段')}</Text>
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
              sourceOptions={sourceOptions}
            />
          ))}
        </div>
      )}
    </Card>
  );
};

export default RiskControlHeadersEditor;

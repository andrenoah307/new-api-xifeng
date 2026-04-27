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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Col,
  Descriptions,
  Divider,
  Empty,
  Input,
  InputNumber,
  TextArea,
  Modal,
  Progress,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  TabPane,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const METRIC_DEFINITIONS = [
  {
    label: '10分钟不同 IP',
    value: 'distinct_ip_10m',
    allowedScopes: ['token', 'user'],
  },
  {
    label: '1小时不同 IP',
    value: 'distinct_ip_1h',
    allowedScopes: ['token', 'user'],
  },
  {
    label: '10分钟不同 UA',
    value: 'distinct_ua_10m',
    allowedScopes: ['token'],
  },
  {
    label: '同 IP 令牌数 (10 分钟)',
    value: 'tokens_per_ip_10m',
    allowedScopes: ['token'],
  },
  {
    label: '1分钟请求数',
    value: 'request_count_1m',
    allowedScopes: ['token', 'user'],
  },
  {
    label: '10分钟请求数',
    value: 'request_count_10m',
    allowedScopes: ['token', 'user'],
  },
  {
    label: '当前并发',
    value: 'inflight_now',
    allowedScopes: ['token', 'user'],
  },
  {
    label: '24小时命中次数',
    value: 'rule_hit_count_24h',
    allowedScopes: ['token', 'user'],
  },
  { label: '可疑度', value: 'risk_score', allowedScopes: ['token', 'user'] },
];

const METRIC_OPTIONS = METRIC_DEFINITIONS.map(({ label, value }) => ({
  label,
  value,
}));

const OP_OPTIONS = [
  { label: '>=', value: '>=' },
  { label: '>', value: '>' },
  { label: '<=', value: '<=' },
  { label: '<', value: '<' },
  { label: '=', value: '==' },
  { label: '!=', value: '!=' },
];

const METRIC_LABEL_MAP = METRIC_OPTIONS.reduce((map, item) => {
  map[item.value] = item.label;
  return map;
}, {});

const METRIC_SCOPE_MAP = METRIC_DEFINITIONS.reduce((map, item) => {
  map[item.value] = item.allowedScopes || [];
  return map;
}, {});

function getMetricOptionsForScope(scope) {
  return METRIC_DEFINITIONS.filter((item) =>
    (item.allowedScopes || []).includes(scope),
  ).map(({ label, value }) => ({ label, value }));
}

function getDefaultMetricForScope(scope) {
  return getMetricOptionsForScope(scope)[0]?.value || 'distinct_ip_10m';
}

function isMetricAllowedForScope(metric, scope) {
  return (METRIC_SCOPE_MAP[metric] || []).includes(scope);
}

function sanitizeConditionsForScope(conditions, scope) {
  const fallbackMetric = getDefaultMetricForScope(scope);
  let changed = false;
  const nextConditions = (conditions || []).map((condition) => {
    if (isMetricAllowedForScope(condition?.metric, scope)) {
      return condition;
    }
    changed = true;
    return {
      ...condition,
      metric: fallbackMetric,
    };
  });
  return { changed, conditions: nextConditions };
}

const emptyRuleForm = () => ({
  id: 0,
  name: '',
  description: '',
  // v4: rules default to disabled because they cannot fire without group bindings.
  enabled: false,
  scope: 'token',
  detector: 'distribution',
  match_mode: 'all',
  priority: 50,
  action: 'observe',
  auto_block: false,
  auto_recover: true,
  recover_mode: 'ttl',
  recover_after_seconds: 900,
  response_status_code: 429,
  response_message: '当前请求触发风控，请稍后再试',
  score_weight: 20,
  conditions: [{ metric: 'distinct_ip_10m', op: '>=', value: 3 }],
  groups: [],
});

function safeParseJSON(value, fallback = []) {
  if (!value) return fallback;
  if (Array.isArray(value)) return value;
  try {
    return JSON.parse(value);
  } catch {
    return fallback;
  }
}

function renderStatus(status) {
  switch (status) {
    case 'blocked':
      return <Tag color='red'>已封禁</Tag>;
    case 'observe':
      return <Tag color='orange'>观察中</Tag>;
    default:
      return <Tag color='grey'>正常</Tag>;
  }
}

function renderDecision(decision) {
  switch (decision) {
    case 'block':
      return <Tag color='red'>封禁</Tag>;
    case 'observe':
      return <Tag color='orange'>观察</Tag>;
    default:
      return <Tag color='grey'>放行</Tag>;
  }
}

function formatRiskCondition(condition) {
  if (!condition) return '-';
  const metricLabel =
    METRIC_LABEL_MAP[condition.metric] || condition.metric || '-';
  return `${metricLabel} ${condition.op || ''} ${condition.value ?? '-'}`;
}

function OverviewCard({ title, value, extra }) {
  return (
    <Card
      bodyStyle={{ padding: 18 }}
      style={{
        borderRadius: 14,
        minHeight: 128,
        border: '1px solid var(--semi-color-border)',
        background: 'var(--semi-color-fill-0)',
      }}
    >
      <Text type='secondary' style={{ fontSize: 12 }}>
        {title}
      </Text>
      <div
        style={{
          fontSize: 30,
          lineHeight: 1.1,
          fontWeight: 700,
          marginTop: 10,
          color: 'var(--semi-color-text-0)',
        }}
      >
        {value}
      </div>
      <div style={{ marginTop: 12 }}>
        <Text type='tertiary' size='small'>
          {extra}
        </Text>
      </div>
    </Card>
  );
}

function RuleEditorModal({
  visible,
  loading,
  initialValue,
  groupOptions,
  enabledGroupSet,
  onCancel,
  onSubmit,
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState(emptyRuleForm());
  const [scopeMetricNotice, setScopeMetricNotice] = useState('');

  useEffect(() => {
    if (!visible) return;
    if (initialValue) {
      const nextForm = {
        ...emptyRuleForm(),
        ...initialValue,
        conditions: safeParseJSON(initialValue.conditions, [
          { metric: 'distinct_ip_10m', op: '>=', value: 3 },
        ]),
        groups: safeParseJSON(initialValue.groups, []),
      };
      const sanitized = sanitizeConditionsForScope(
        nextForm.conditions,
        nextForm.scope,
      );
      setForm({
        ...nextForm,
        conditions: sanitized.conditions,
      });
      setScopeMetricNotice(
        sanitized.changed
          ? t('部分指标不适用于当前作用域，系统已自动替换为 {{metric}}。', {
              metric:
                METRIC_LABEL_MAP[getDefaultMetricForScope(nextForm.scope)],
            })
          : '',
      );
      return;
    }
    setForm(emptyRuleForm());
    setScopeMetricNotice('');
  }, [visible, initialValue]);

  const updateField = (field, value) => {
    if (field === 'scope') {
      const sanitized = sanitizeConditionsForScope(form.conditions, value);
      setForm((prev) => ({
        ...prev,
        scope: value,
        conditions: sanitized.conditions,
      }));
      setScopeMetricNotice(
        sanitized.changed
          ? t(
              '已切换为 {{scope}} 作用域，不兼容的指标已自动替换为 {{metric}}。',
              {
                scope: value === 'token' ? 'API Key' : t('用户'),
                metric: METRIC_LABEL_MAP[getDefaultMetricForScope(value)],
              },
            )
          : '',
      );
      return;
    }
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const updateCondition = (index, field, value) => {
    setForm((prev) => ({
      ...prev,
      conditions: prev.conditions.map((item, itemIndex) =>
        itemIndex === index ? { ...item, [field]: value } : item,
      ),
    }));
  };

  const addCondition = () => {
    setForm((prev) => ({
      ...prev,
      conditions: [
        ...prev.conditions,
        { metric: getDefaultMetricForScope(prev.scope), op: '>=', value: 1 },
      ],
    }));
  };

  const availableMetricOptions = getMetricOptionsForScope(form.scope);

  const removeCondition = (index) => {
    setForm((prev) => ({
      ...prev,
      conditions: prev.conditions.filter((_, itemIndex) => itemIndex !== index),
    }));
  };

  const handleSubmit = () => {
    if (!form.name.trim()) {
      return showError(t('规则名称不能为空'));
    }
    if (!form.conditions.length) {
      return showError(t('至少需要一个条件'));
    }
    // v4 invariant: enabled rules must be bound to at least one group, otherwise
    // the engine silently skips them (DEV_GUIDE §5).
    if (form.enabled && (!form.groups || form.groups.length === 0)) {
      return showError(t('启用规则前必须至少选择一个分组'));
    }
    onSubmit(form);
  };

  const sectionStyle = {
    background: 'var(--semi-color-fill-0)',
    borderRadius: 10,
    padding: '14px 16px',
    width: '100%',
  };

  return (
    <Modal
      title={form.id ? t('编辑风控规则') : t('新建风控规则')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      okText={t('保存')}
      cancelText={t('取消')}
      confirmLoading={loading}
      width={860}
      centered
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
    >
      <Space vertical align='start' style={{ width: '100%' }} spacing='medium'>
        {scopeMetricNotice ? (
          <Banner
            type='warning'
            closeIcon={null}
            description={scopeMetricNotice}
          />
        ) : null}

        {/* 基础信息 */}
        <div style={sectionStyle}>
          <Text strong style={{ display: 'block', marginBottom: 12 }}>
            {t('基础信息')}
          </Text>
          <Row gutter={[16, 16]}>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('规则名称')}
              </Text>
              <Input
                value={form.name}
                onChange={(value) => updateField('name', value)}
                placeholder={t('例如 token_multi_ip_block')}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('作用域')}
              </Text>
              <Select
                value={form.scope}
                style={{ width: '100%' }}
                onChange={(value) => updateField('scope', value)}
                optionList={[
                  { label: t('API Key'), value: 'token' },
                  { label: t('用户'), value: 'user' },
                ]}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('检测器')}
              </Text>
              <Select
                value={form.detector}
                style={{ width: '100%' }}
                onChange={(value) => updateField('detector', value)}
                optionList={[{ label: t('分发检测'), value: 'distribution' }]}
              />
            </Col>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('规则描述')}
              </Text>
              <TextArea
                value={form.description}
                rows={2}
                maxCount={200}
                onChange={(value) => updateField('description', value)}
                placeholder={t('简要说明此规则的风险目标和作用')}
              />
            </Col>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('适用分组')}
              </Text>
              <Select
                multiple
                style={{ width: '100%' }}
                value={form.groups || []}
                onChange={(value) => updateField('groups', value || [])}
                placeholder={t('选择规则生效的分组')}
                optionList={(groupOptions || []).map((name) => ({
                  label:
                    enabledGroupSet && enabledGroupSet.has(name)
                      ? name
                      : `${name} ${t('(分组未启用风控)')}`,
                  value: name,
                }))}
                getPopupContainer={() => document.body}
              />
              <Text
                type='tertiary'
                size='small'
                style={{ display: 'block', marginTop: 6 }}
              >
                {t(
                  '未配置分组的规则不会生效。仅在分组被加入风控白名单后规则才会真正参与评估。',
                )}
              </Text>
            </Col>
          </Row>
        </div>

        {/* 规则条件 */}
        <div style={sectionStyle}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 12,
            }}
          >
            <Text strong>{t('规则条件')}</Text>
            <Button type='tertiary' size='small' onClick={addCondition}>
              {t('添加条件')}
            </Button>
          </div>
          <Row gutter={[16, 16]}>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('匹配方式')}
              </Text>
              <Select
                value={form.match_mode}
                style={{ width: '100%' }}
                onChange={(value) => updateField('match_mode', value)}
                optionList={[
                  { label: t('全部满足 (AND)'), value: 'all' },
                  { label: t('任一满足 (OR)'), value: 'any' },
                ]}
              />
            </Col>
          </Row>
          <Space
            vertical
            style={{ width: '100%', marginTop: 12 }}
            spacing='tight'
          >
            {form.conditions.map((condition, index) => (
              <div
                key={`condition-${index}`}
                style={{
                  borderRadius: 8,
                  padding: '10px 12px',
                  border: '1px solid var(--semi-color-border)',
                  background: 'var(--semi-color-bg-1)',
                }}
              >
                <Row gutter={12} type='flex' align='middle'>
                  <Col span={9}>
                    <Select
                      value={condition.metric}
                      optionList={availableMetricOptions}
                      style={{ width: '100%' }}
                      onChange={(value) =>
                        updateCondition(index, 'metric', value)
                      }
                    />
                  </Col>
                  <Col span={5}>
                    <Select
                      value={condition.op}
                      optionList={OP_OPTIONS}
                      style={{ width: '100%' }}
                      onChange={(value) => updateCondition(index, 'op', value)}
                    />
                  </Col>
                  <Col span={6}>
                    <InputNumber
                      value={condition.value}
                      style={{ width: '100%' }}
                      onChange={(value) =>
                        updateCondition(index, 'value', value || 0)
                      }
                    />
                  </Col>
                  <Col span={4} style={{ textAlign: 'right' }}>
                    <Button
                      type='danger'
                      theme='borderless'
                      size='small'
                      onClick={() => removeCondition(index)}
                      disabled={form.conditions.length === 1}
                    >
                      {t('删除')}
                    </Button>
                  </Col>
                </Row>
              </div>
            ))}
          </Space>
        </div>

        {/* 触发行为 */}
        <div style={sectionStyle}>
          <Text strong style={{ display: 'block', marginBottom: 12 }}>
            {t('触发行为')}
          </Text>
          <Row gutter={[16, 16]}>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('动作')}
              </Text>
              <Select
                value={form.action}
                style={{ width: '100%' }}
                onChange={(value) => updateField('action', value)}
                optionList={[
                  { label: t('观察'), value: 'observe' },
                  { label: t('封禁'), value: 'block' },
                ]}
              />
            </Col>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('优先级')}
              </Text>
              <InputNumber
                value={form.priority}
                min={0}
                max={999}
                style={{ width: '100%' }}
                onChange={(value) => updateField('priority', value || 0)}
              />
            </Col>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('可疑度权重')}
              </Text>
              <InputNumber
                value={form.score_weight}
                min={0}
                max={100}
                style={{ width: '100%' }}
                onChange={(value) => updateField('score_weight', value || 0)}
              />
            </Col>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('返回状态码')}
              </Text>
              <InputNumber
                value={form.response_status_code}
                min={200}
                max={599}
                style={{ width: '100%' }}
                onChange={(value) =>
                  updateField('response_status_code', value || 429)
                }
              />
            </Col>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('返回消息')}
              </Text>
              <Input
                value={form.response_message}
                onChange={(value) => updateField('response_message', value)}
                placeholder={t('当前请求触发风控，请稍后再试')}
              />
            </Col>
          </Row>
        </div>

        {/* 恢复策略 */}
        <div style={sectionStyle}>
          <Text strong style={{ display: 'block', marginBottom: 12 }}>
            {t('恢复策略')}
          </Text>
          <Row gutter={[16, 16]}>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('规则启用')}
              </Text>
              <Switch
                checked={form.enabled}
                onChange={(value) => updateField('enabled', value)}
              />
            </Col>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('自动封禁')}
              </Text>
              <Switch
                checked={form.auto_block}
                onChange={(value) => updateField('auto_block', value)}
                disabled={form.action !== 'block'}
              />
            </Col>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('自动恢复')}
              </Text>
              <Switch
                checked={form.auto_recover}
                onChange={(value) => updateField('auto_recover', value)}
              />
            </Col>
            <Col span={6}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('恢复方式')}
              </Text>
              <Select
                value={form.recover_mode}
                style={{ width: '100%' }}
                onChange={(value) => updateField('recover_mode', value)}
                optionList={[
                  { label: t('TTL 自动恢复'), value: 'ttl' },
                  { label: t('人工恢复'), value: 'manual' },
                ]}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('恢复时间（秒）')}
              </Text>
              <InputNumber
                value={form.recover_after_seconds}
                min={60}
                max={86400 * 7}
                style={{ width: '100%' }}
                onChange={(value) =>
                  updateField('recover_after_seconds', value || 900)
                }
              />
            </Col>
          </Row>
        </div>
      </Space>
    </Modal>
  );
}

// emptyModerationRuleForm seeds the editor with sane defaults — match_mode
// "all" is the conservative pick (every condition must satisfy) so admins
// who skim past the toggle don't accidentally configure broad triggers.
const emptyModerationRuleForm = () => ({
  id: 0,
  name: '',
  description: '',
  enabled: false,
  match_mode: 'all',
  action: 'observe',
  priority: 50,
  score_weight: 20,
  conditions: [
    {
      category: 'sexual',
      op: '>=',
      value: 0.5,
      apply_input_type: false,
      applied_input_type: '',
    },
  ],
  groups: [],
});

// ModerationRuleEditorModal mirrors the structure of RuleEditorModal so admins
// don't need a separate mental model. Each condition row exposes a toggle
// "限定输入类型" — when off the score is taken from any input modality;
// when on the admin picks text or image and the rule only fires when the
// upstream attributed the score to that input.
function ModerationRuleEditorModal({
  visible,
  loading,
  initialValue,
  groupOptions,
  enabledGroupSet,
  categories,
  onCancel,
  onSubmit,
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState(emptyModerationRuleForm());

  useEffect(() => {
    if (!visible) return;
    if (initialValue) {
      setForm({
        ...emptyModerationRuleForm(),
        ...initialValue,
        conditions: safeParseJSON(initialValue.conditions, [
          {
            category: 'sexual',
            op: '>=',
            value: 0.5,
            apply_input_type: false,
            applied_input_type: '',
          },
        ]),
        groups: safeParseJSON(initialValue.groups, []),
      });
      return;
    }
    setForm(emptyModerationRuleForm());
  }, [visible, initialValue]);

  const updateField = (field, value) =>
    setForm((p) => ({ ...p, [field]: value }));

  const updateCondition = (index, field, value) =>
    setForm((p) => ({
      ...p,
      conditions: p.conditions.map((c, i) =>
        i === index ? { ...c, [field]: value } : c,
      ),
    }));

  const addCondition = () =>
    setForm((p) => ({
      ...p,
      conditions: [
        ...p.conditions,
        {
          category: categories[0]?.name || 'sexual',
          op: '>=',
          value: 0.5,
          apply_input_type: false,
          applied_input_type: '',
        },
      ],
    }));

  const removeCondition = (index) =>
    setForm((p) => ({
      ...p,
      conditions: p.conditions.filter((_, i) => i !== index),
    }));

  const submit = () => {
    if (!form.name.trim()) return showError(t('规则名称不能为空'));
    if (!form.conditions.length) return showError(t('至少需要一个条件'));
    if (form.enabled && (!form.groups || form.groups.length === 0)) {
      return showError(t('启用规则前必须至少选择一个分组'));
    }
    onSubmit(form);
  };

  const categoryOptions = (categories || []).map((c) => ({
    label: c.label,
    value: c.name,
  }));

  const sectionStyle = {
    background: 'var(--semi-color-fill-0)',
    borderRadius: 10,
    padding: '14px 16px',
    width: '100%',
  };

  return (
    <Modal
      title={form.id ? t('编辑审核规则') : t('新建审核规则')}
      visible={visible}
      onCancel={onCancel}
      onOk={submit}
      okText={t('保存')}
      cancelText={t('取消')}
      confirmLoading={loading}
      width={860}
      centered
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
    >
      <Space vertical align='start' style={{ width: '100%' }} spacing='medium'>
        <div style={sectionStyle}>
          <Text strong style={{ display: 'block', marginBottom: 12 }}>
            {t('基础信息')}
          </Text>
          <Row gutter={[16, 16]}>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('规则名称')}
              </Text>
              <Input
                value={form.name}
                onChange={(v) => updateField('name', v)}
                placeholder={t('例如 sexual_minors_block')}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('匹配方式')}
              </Text>
              <Select
                style={{ width: '100%' }}
                value={form.match_mode}
                onChange={(v) => updateField('match_mode', v)}
                optionList={[
                  { label: t('全部满足 (AND)'), value: 'all' },
                  { label: t('任一满足 (OR)'), value: 'any' },
                ]}
                getPopupContainer={() => document.body}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('动作')}
              </Text>
              <Select
                style={{ width: '100%' }}
                value={form.action}
                onChange={(v) => updateField('action', v)}
                optionList={[
                  { label: t('观察 (observe)'), value: 'observe' },
                  { label: t('标记 (flag)'), value: 'flag' },
                  {
                    label: t('阻断 (block) — 与 flag 等效，预留'),
                    value: 'block',
                  },
                ]}
                getPopupContainer={() => document.body}
              />
            </Col>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('描述')}
              </Text>
              <TextArea
                rows={2}
                value={form.description}
                onChange={(v) => updateField('description', v)}
                placeholder={t('简要说明规则用途')}
              />
            </Col>
            <Col span={24}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('适用分组')}
              </Text>
              <Select
                multiple
                style={{ width: '100%' }}
                value={form.groups || []}
                onChange={(v) => updateField('groups', v || [])}
                optionList={(groupOptions || []).map((name) => ({
                  label:
                    enabledGroupSet && enabledGroupSet.has(name)
                      ? name
                      : `${name} ${t('(分组未启用内容审核)')}`,
                  value: name,
                }))}
                getPopupContainer={() => document.body}
              />
              <Text
                type='tertiary'
                size='small'
                style={{ display: 'block', marginTop: 6 }}
              >
                {t(
                  '未绑定分组的规则不会生效；分组未在白名单中时规则不会被加载。',
                )}
              </Text>
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('启用')}
              </Text>
              <Switch
                checked={!!form.enabled}
                onChange={(v) => updateField('enabled', v)}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('优先级')}
              </Text>
              <InputNumber
                value={form.priority}
                onChange={(v) => updateField('priority', v || 0)}
                style={{ width: '100%' }}
              />
            </Col>
            <Col span={8}>
              <Text strong style={{ display: 'block', marginBottom: 6 }}>
                {t('权重')}
              </Text>
              <InputNumber
                value={form.score_weight}
                onChange={(v) => updateField('score_weight', v || 0)}
                style={{ width: '100%' }}
              />
            </Col>
          </Row>
        </div>

        <div style={sectionStyle}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 12,
            }}
          >
            <Text strong>{t('规则条件')}</Text>
            <Button type='tertiary' size='small' onClick={addCondition}>
              {t('添加条件')}
            </Button>
          </div>
          <Space vertical style={{ width: '100%' }} spacing='tight'>
            {form.conditions.map((c, idx) => {
              const catDef = (categories || []).find(
                (x) => x.name === c.category,
              );
              const imageScored = !!catDef?.image_scored;
              return (
                <div
                  key={idx}
                  style={{
                    borderRadius: 8,
                    padding: '10px 12px',
                    border: '1px solid var(--semi-color-border)',
                    background: 'var(--semi-color-bg-1)',
                  }}
                >
                  <Row gutter={12} type='flex' align='middle'>
                    <Col span={8}>
                      <Text type='secondary' size='small'>
                        {t('类别')}
                      </Text>
                      <Select
                        value={c.category}
                        optionList={categoryOptions}
                        style={{ width: '100%' }}
                        onChange={(v) => updateCondition(idx, 'category', v)}
                        getPopupContainer={() => document.body}
                      />
                    </Col>
                    <Col span={4}>
                      <Text type='secondary' size='small'>
                        {t('比较')}
                      </Text>
                      <Select
                        value={c.op}
                        optionList={OP_OPTIONS}
                        style={{ width: '100%' }}
                        onChange={(v) => updateCondition(idx, 'op', v)}
                        getPopupContainer={() => document.body}
                      />
                    </Col>
                    <Col span={4}>
                      <Text type='secondary' size='small'>
                        {t('阈值')}
                      </Text>
                      <InputNumber
                        min={0}
                        max={1}
                        step={0.05}
                        value={c.value}
                        style={{ width: '100%' }}
                        onChange={(v) =>
                          updateCondition(
                            idx,
                            'value',
                            typeof v === 'number' ? v : 0,
                          )
                        }
                      />
                    </Col>
                    <Col span={5}>
                      <Text type='secondary' size='small'>
                        {t('限定输入类型')}
                      </Text>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 8,
                        }}
                      >
                        <Switch
                          checked={!!c.apply_input_type}
                          onChange={(v) =>
                            updateCondition(idx, 'apply_input_type', v)
                          }
                        />
                        {c.apply_input_type ? (
                          <Select
                            value={c.applied_input_type || 'text'}
                            style={{ flex: 1 }}
                            onChange={(v) =>
                              updateCondition(idx, 'applied_input_type', v)
                            }
                            getPopupContainer={() => document.body}
                            optionList={[
                              { label: t('文本'), value: 'text' },
                              {
                                label: t('图像'),
                                value: 'image',
                                disabled: !imageScored,
                              },
                            ]}
                          />
                        ) : null}
                      </div>
                    </Col>
                    <Col span={3} style={{ textAlign: 'right' }}>
                      <Button
                        type='danger'
                        theme='borderless'
                        size='small'
                        onClick={() => removeCondition(idx)}
                        disabled={form.conditions.length === 1}
                      >
                        {t('删除')}
                      </Button>
                    </Col>
                  </Row>
                </div>
              );
            })}
          </Space>
        </div>
      </Space>
    </Modal>
  );
}

// ModerationTab is the second top-level tab on /console/risk. It owns its
// own state because the moderation engine is decoupled from the
// distribution-detection engine on the backend; sharing state here would only
// introduce coupling for the sake of saving a few lines.
function ModerationTab({ riskGroups }) {
  const { t } = useTranslation();
  const [config, setConfig] = useState({
    enabled: false,
    mode: 'off',
    base_url: 'https://api.openai.com',
    model: 'omni-moderation-latest',
    api_keys: [],
    sampling_rate_percent: 100,
    enabled_groups: [],
    group_modes: {},
    flagged_retention_hours: 720,
    benign_retention_hours: 72,
    record_unmatched_inputs: false,
    event_queue_size: 4096,
    worker_count: 2,
    http_timeout_ms: 5000,
    max_retries: 3,
    image_max_size_kb: 2048,
    debug_result_retain_minutes: 10,
  });
  const [overview, setOverview] = useState({});
  const [keyCount, setKeyCount] = useState(0);
  const [savingConfig, setSavingConfig] = useState(false);
  const [keysInput, setKeysInput] = useState('');
  // Debug card state
  const [debugText, setDebugText] = useState('');
  const [debugImages, setDebugImages] = useState('');
  const [debugRunning, setDebugRunning] = useState(false);
  const [debugResult, setDebugResult] = useState(null);
  const [debugError, setDebugError] = useState('');
  // debugGroup: the group context used to evaluate rules during a debug run.
  // Empty string => preview mode (every enabled rule, no group filter); a
  // group name => evaluate against that group's bound rules, mirroring how
  // production traffic in that group would be judged.
  const [debugGroup, setDebugGroup] = useState('');
  // Incidents
  const [incidents, setIncidents] = useState([]);
  const [incidentsPage, setIncidentsPage] = useState({
    page: 1,
    page_size: 10,
    total: 0,
  });
  const [incidentFilters, setIncidentFilters] = useState({
    group: '',
    flagged: '',
    keyword: '',
  });
  // Live runtime stats — populated by GET /api/risk/moderation/queue_stats
  // and polled every 15s while this tab is visible. Polling pauses on
  // unmount via the cleanup return.
  const [queueStats, setQueueStats] = useState(null);

  // Detail modal for viewing full input content of an incident
  const [detailIncidentId, setDetailIncidentId] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [detailData, setDetailData] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);

  // Moderation rules
  const [moderationRules, setModerationRules] = useState([]);
  const [moderationCategories, setModerationCategories] = useState([]);
  const [ruleEditorVisible, setRuleEditorVisible] = useState(false);
  const [editingModerationRule, setEditingModerationRule] = useState(null);
  const [savingModerationRule, setSavingModerationRule] = useState(false);

  // enabledGroupSet — every "is this group enabled for content moderation"
  // decision inside this tab MUST consult the moderation engine's own
  // EnabledGroups, not the risk-control whitelist. The two engines are
  // decoupled (DEV_GUIDE §8) so an admin can perfectly enable a group for
  // moderation while leaving distribution detection off, and vice versa.
  // Earlier versions wrongly read riskGroups (risk-control's view) and
  // produced contradictory labels — e.g. the matrix showed default = ON
  // but the debug dropdown still tagged default as "未启用内容审核".
  const enabledGroupSet = useMemo(() => {
    const s = new Set();
    for (const g of config.enabled_groups || []) {
      s.add(g);
    }
    return s;
  }, [config.enabled_groups]);

  const loadConfig = async () => {
    const res = await API.get('/api/risk/moderation/config');
    if (res.data.success) {
      const cfg = res.data.data?.config || {};
      setConfig((prev) => ({ ...prev, ...cfg }));
      setKeyCount(res.data.data?.key_count || 0);
      // pre-fill the keys textarea with the masked entries so admins can
      // see how many keys are currently configured without exposing them.
      setKeysInput((cfg.api_keys || []).join('\n'));
    }
  };

  const loadOverview = async () => {
    const res = await API.get('/api/risk/moderation/overview');
    if (res.data.success) setOverview(res.data.data || {});
  };

  const loadIncidents = async (
    page = incidentsPage.page,
    pageSize = incidentsPage.page_size,
    filters = incidentFilters,
  ) => {
    const params = { p: page, page_size: pageSize };
    if (filters.group) params.group = filters.group;
    if (filters.flagged !== '' && filters.flagged !== undefined)
      params.flagged = filters.flagged;
    if (filters.keyword) params.keyword = filters.keyword;
    const res = await API.get('/api/risk/moderation/incidents', { params });
    if (res.data.success) {
      const data = res.data.data || {};
      setIncidents(data.items || []);
      setIncidentsPage({
        page: data.page || page,
        page_size: data.page_size || pageSize,
        total: data.total || 0,
      });
    }
  };

  const loadModerationRules = async () => {
    const res = await API.get('/api/risk/moderation/rules');
    if (res.data.success) setModerationRules(res.data.data || []);
  };

  const loadModerationCategories = async () => {
    const res = await API.get('/api/risk/moderation/categories');
    if (res.data.success) setModerationCategories(res.data.data || []);
  };

  const loadQueueStats = async () => {
    try {
      const res = await API.get('/api/risk/moderation/queue_stats');
      if (res.data.success) setQueueStats(res.data.data || null);
    } catch (e) {
      // silent — the stats card is informational, errors should not
      // disrupt the rest of the tab.
    }
  };

  useEffect(() => {
    loadQueueStats();
    const id = setInterval(loadQueueStats, 15000);
    return () => clearInterval(id);
  }, []);

  const handleSaveModerationRule = async (form) => {
    setSavingModerationRule(true);
    try {
      const payload = {
        ...form,
        conditions: form.conditions || [],
        groups: form.groups || [],
      };
      const res = form.id
        ? await API.put(`/api/risk/moderation/rules/${form.id}`, payload)
        : await API.post('/api/risk/moderation/rules', payload);
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('审核规则已保存'));
      setRuleEditorVisible(false);
      setEditingModerationRule(null);
      await Promise.all([loadModerationRules(), loadOverview()]);
    } catch (e) {
      showError(t('保存审核规则失败'));
    } finally {
      setSavingModerationRule(false);
    }
  };

  const handleDeleteModerationRule = (rule) => {
    Modal.confirm({
      title: t('确认删除规则'),
      content: `${t('规则')}: ${rule?.name || '-'}`,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/risk/moderation/rules/${rule.id}`);
          if (!res.data.success) {
            showError(res.data.message);
            return;
          }
          showSuccess(t('规则已删除'));
          await Promise.all([loadModerationRules(), loadOverview()]);
        } catch (e) {
          showError(t('删除规则失败'));
        }
      },
    });
  };

  const handleToggleModerationRule = async (rule, enabled) => {
    const groups = safeParseJSON(rule.groups, []);
    if (enabled && (!groups || groups.length === 0)) {
      showError(t('启用规则前必须至少选择一个分组'));
      return;
    }
    await handleSaveModerationRule({
      ...rule,
      conditions: safeParseJSON(rule.conditions, []),
      groups,
      enabled,
    });
  };

  useEffect(() => {
    loadConfig();
    loadOverview();
    loadIncidents(1);
    loadModerationRules();
    loadModerationCategories();
  }, []);

  useEffect(() => {
    if (!detailModalVisible || !detailIncidentId) return;
    setDetailLoading(true);
    API.get(`/api/risk/moderation/incidents/${detailIncidentId}`)
      .then((res) => {
        if (res.data?.success) {
          setDetailData(res.data.data);
        }
      })
      .finally(() => setDetailLoading(false));
  }, [detailModalVisible, detailIncidentId]);

  const handleSaveConfig = async () => {
    setSavingConfig(true);
    try {
      const payload = {
        ...config,
        // Split textarea into lines; backend mergePreservedModerationKeys
        // will substitute masked entries with the existing real keys.
        api_keys: keysInput
          .split('\n')
          .map((s) => s.trim())
          .filter((s) => s),
        preserve_existing_keys: true,
      };
      const res = await API.put('/api/risk/moderation/config', payload);
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('内容审核配置已保存'));
      const cfg = res.data.data?.config || {};
      setConfig((prev) => ({ ...prev, ...cfg }));
      setKeyCount(res.data.data?.key_count || 0);
      setKeysInput((cfg.api_keys || []).join('\n'));
      await loadOverview();
    } catch (e) {
      showError(t('保存内容审核配置失败'));
    } finally {
      setSavingConfig(false);
    }
  };

  const toggleModerationGroup = (name, enabled) => {
    setConfig((prev) => {
      const list = Array.isArray(prev.enabled_groups)
        ? [...prev.enabled_groups]
        : [];
      const next = list.filter((g) => g !== name);
      if (enabled) next.push(name);
      return { ...prev, enabled_groups: next };
    });
  };

  const setGroupMode = (name, mode) => {
    setConfig((prev) => {
      const modes = { ...(prev.group_modes || {}) };
      if (mode === '__delete__') delete modes[name];
      else modes[name] = mode;
      return { ...prev, group_modes: modes };
    });
  };

  const runDebug = async () => {
    setDebugRunning(true);
    setDebugError('');
    setDebugResult(null);
    try {
      const images = debugImages
        .split('\n')
        .map((s) => s.trim())
        .filter((s) => s);
      const res = await API.post('/api/risk/moderation/debug', {
        text: debugText,
        images,
        group: debugGroup || '',
      });
      if (!res.data.success) {
        setDebugError(res.data.message || t('提交失败'));
        setDebugRunning(false);
        return;
      }
      const requestID = res.data.data?.request_id;
      // Poll up to 30s, 1s interval. Server side uses the same async worker
      // pool so the debug job competes fairly with relay traffic; if the
      // queue is busy the poller times out and the admin can retry.
      const start = Date.now();
      const poll = async () => {
        if (Date.now() - start > 30000) {
          setDebugError(t('检测超时，请稍后重试'));
          setDebugRunning(false);
          return;
        }
        try {
          const r = await API.get(
            `/api/risk/moderation/debug/${encodeURIComponent(requestID)}`,
          );
          if (r.data.success && r.data.data?.pending === false) {
            setDebugResult(r.data.data.result);
            setDebugRunning(false);
            return;
          }
        } catch (e) {
          // transient — keep polling
        }
        setTimeout(poll, 1000);
      };
      poll();
    } catch (e) {
      setDebugError(t('提交失败'));
      setDebugRunning(false);
    }
  };

  const incidentColumns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      render: (v) => <Tag color='cyan'>{v || '-'}</Tag>,
    },
    {
      title: t('用户'),
      dataIndex: 'username',
      render: (_, r) => (
        <Space vertical spacing={2}>
          <Text>{r.username || '-'}</Text>
          <Text type='secondary' size='small'>
            UID {r.user_id || '-'}
          </Text>
        </Space>
      ),
    },
    {
      title: t('API Key'),
      dataIndex: 'token_name',
      render: (_, r) => (
        <Space vertical spacing={2}>
          <Text>{r.token_name || '-'}</Text>
          <Text type='secondary' size='small'>
            {r.token_masked_key || '-'}
          </Text>
        </Space>
      ),
    },
    {
      title: t('结果'),
      dataIndex: 'flagged',
      render: (v) =>
        v ? (
          <Tag color='red'>{t('命中')}</Tag>
        ) : (
          <Tag color='grey'>{t('未命中')}</Tag>
        ),
    },
    {
      title: t('最高分'),
      dataIndex: 'max_score',
      render: (v) => (typeof v === 'number' ? v.toFixed(3) : '-'),
    },
    { title: t('最高类别'), dataIndex: 'max_category' },
    { title: t('来源'), dataIndex: 'source' },
    { title: t('上游耗时(ms)'), dataIndex: 'upstream_latency_ms' },
    {
      title: (
        <Tooltip content={t('取自用户请求的最后一条消息内容，可能包含客户端注入的协议标签，属正常现象')}>
          <span style={{ cursor: 'help', borderBottom: '1px dashed var(--semi-color-text-2)' }}>
            {t('输入摘要')}
          </span>
        </Tooltip>
      ),
      dataIndex: 'input_summary',
      render: (v, record) => (
        <Text
          ellipsis={{ showTooltip: false }}
          style={{ maxWidth: '30vw', cursor: v ? 'pointer' : 'default' }}
          onClick={() => {
            if (v) {
              setDetailIncidentId(record.id);
              setDetailModalVisible(true);
            }
          }}
        >
          {v || '-'}
        </Text>
      ),
    },
  ];

  return (
    <div className='flex flex-col gap-3'>
      <Banner
        type='info'
        closeIcon={null}
        description={t(
          '内容审核默认对所有分组关闭。所有调用都是异步且不阻塞主链路；建议先观察打分，确认阈值合适后再考虑后续动作。',
        )}
      />

      {/* Live runtime stats card — refreshed every 15s. Shows queue depth
          (memory + Redis), per-worker state, dropped events, and the
          incident batcher backlog. */}
      <Card bodyStyle={{ padding: 16 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <Title heading={6} style={{ marginTop: 0 }}>
            {t('运行状态（每 15 秒自动刷新）')}
          </Title>
          <Space>
            <Button onClick={loadQueueStats}>{t('立即刷新')}</Button>
          </Space>
        </div>
        {queueStats ? (
          <div style={{ marginTop: 8 }}>
            <Space wrap>
              <Tag color='blue'>
                {t('内存队列')}: {queueStats.queue_depth_memory ?? 0}
              </Tag>
              <Tag color={queueStats.redis_available ? 'cyan' : 'grey'}>
                {t('Redis 队列')}:{' '}
                {queueStats.redis_available
                  ? (queueStats.queue_depth_redis ?? 0)
                  : t('未启用')}
              </Tag>
              <Tag color='orange'>
                {t('事件丢弃累计')}: {queueStats.drop_count_total ?? 0}
              </Tag>
              <Tag color='green'>
                {t('Worker 总数')}: {queueStats.worker_count ?? 0}
              </Tag>
              {queueStats.incident_batcher ? (
                <Tag color='purple'>
                  {t('待写入审计')}: {queueStats.incident_batcher.pending ?? 0}{' '}
                  / {t('累计写入')}:{' '}
                  {queueStats.incident_batcher.total_flushed ?? 0}
                </Tag>
              ) : null}
            </Space>
            <div style={{ marginTop: 10 }}>
              <Text type='secondary' size='small'>
                {t('Worker 状态')}：
              </Text>
              <Space wrap style={{ marginTop: 4 }}>
                {(queueStats.worker_state || []).map((w) => (
                  <Tag
                    key={w.id}
                    color={w.state === 'processing' ? 'orange' : 'green'}
                    size='small'
                  >
                    #{w.id} {w.state === 'processing' ? t('处理中') : t('空闲')}
                  </Tag>
                ))}
              </Space>
            </div>
          </div>
        ) : (
          <Text type='tertiary' size='small'>
            {t('正在加载运行状态…')}
          </Text>
        )}
      </Card>

      {/* Overview cards */}
      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <Row gutter={[12, 12]}>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('启用状态')}
              value={overview.enabled ? t('已启用') : t('未启用')}
              extra={`${t('全局模式')}: ${overview.mode || 'off'}`}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('已配置 API Key 数')}
              value={overview.key_count || 0}
              extra={t('多 key 轮询，触发限流自动切换')}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('24h 命中数')}
              value={overview.flagged_24h || 0}
              extra={`${t('规则数')}: ${overview.rule_count ?? 0} / ${t('未配置')}: ${overview.unconfigured_rule_count ?? 0}`}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('事件丢弃')}
              value={overview.queue_dropped || 0}
              extra={`${t('采样率')}: ${overview.sampling_rate_percent ?? 100}%`}
            />
          </Col>
        </Row>
      </Card>

      {/* Global config */}
      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <div>
            <Title heading={5} style={{ marginTop: 0 }}>
              {t('内容审核全局策略')}
            </Title>
            <Text type='secondary'>
              {t(
                '调用 OpenAI omni-moderation 模型进行异步内容评分；命中阈值的事件保留供下游处置使用。',
              )}
            </Text>
          </div>
          <Button
            type='primary'
            loading={savingConfig}
            onClick={handleSaveConfig}
          >
            {t('保存内容审核配置')}
          </Button>
        </div>
        <Row gutter={[12, 12]} style={{ marginTop: 14 }}>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('开启内容审核')}</Text>
            <div style={{ marginTop: 10 }}>
              <Switch
                checked={!!config.enabled}
                onChange={(v) => setConfig((p) => ({ ...p, enabled: v }))}
              />
            </div>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('全局模式')}</Text>
            <Select
              value={config.mode}
              style={{ width: '100%' }}
              onChange={(v) => setConfig((p) => ({ ...p, mode: v }))}
              optionList={[
                { label: t('关闭'), value: 'off' },
                { label: t('观察模式'), value: 'observe_only' },
                {
                  label: t('执行模式（预留，本期等同观察）'),
                  value: 'enforce',
                },
              ]}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('OpenAI Base URL')}</Text>
            <Input
              value={config.base_url}
              onChange={(v) => setConfig((p) => ({ ...p, base_url: v }))}
              placeholder='https://api.openai.com'
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('模型名')}</Text>
            <Input
              value={config.model}
              onChange={(v) => setConfig((p) => ({ ...p, model: v }))}
              placeholder='omni-moderation-latest'
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('采样率（%）')}</Text>
            <InputNumber
              min={0}
              max={100}
              value={config.sampling_rate_percent}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  sampling_rate_percent: typeof v === 'number' ? v : 100,
                }))
              }
              style={{ width: '100%' }}
              suffix='%'
            />
          </Col>
          {/* v3: 兜底阈值已移除，命中由审核规则系统决定 */}
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('队列大小')}</Text>
            <InputNumber
              min={64}
              value={config.event_queue_size}
              onChange={(v) =>
                setConfig((p) => ({ ...p, event_queue_size: v || 4096 }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('Worker 数')}</Text>
            <InputNumber
              min={1}
              max={32}
              value={config.worker_count}
              onChange={(v) =>
                setConfig((p) => ({ ...p, worker_count: v || 2 }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('HTTP 超时（ms）')}</Text>
            <InputNumber
              min={500}
              value={config.http_timeout_ms}
              onChange={(v) =>
                setConfig((p) => ({ ...p, http_timeout_ms: v || 5000 }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('429/5xx 重试次数')}</Text>
            <InputNumber
              min={0}
              max={10}
              value={config.max_retries}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  max_retries: typeof v === 'number' ? v : 3,
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('命中保留（小时）')}</Text>
            <InputNumber
              min={1}
              value={config.flagged_retention_hours}
              onChange={(v) =>
                setConfig((p) => ({ ...p, flagged_retention_hours: v || 720 }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('未命中保留（小时）')}</Text>
            <InputNumber
              min={1}
              value={config.benign_retention_hours}
              onChange={(v) =>
                setConfig((p) => ({ ...p, benign_retention_hours: v || 72 }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('记录未命中输入')}</Text>
            <div style={{ marginTop: 6 }}>
              <Switch
                checked={config.record_unmatched_inputs}
                onChange={(v) =>
                  setConfig((p) => ({ ...p, record_unmatched_inputs: v }))
                }
              />
              <Text type='tertiary' size='small' style={{ marginLeft: 8 }}>
                {t('关闭时仅记录命中规则的请求，降低数据库压力')}
              </Text>
            </div>
          </Col>
          <Col xs={24}>
            <Text strong>
              {t(
                'OpenAI API Keys（每行一个，已存在的 key 显示为掩码，保存时保持不变）',
              )}
            </Text>
            <TextArea
              value={keysInput}
              onChange={(v) => setKeysInput(v)}
              rows={4}
              placeholder={t('每行一个 API Key；触发 RPM 限制时按顺序自动切换')}
              style={{ marginTop: 6 }}
            />
            <Text type='tertiary' size='small'>
              {t('当前已配置 {{n}} 个 key', { n: keyCount })}
            </Text>
          </Col>
        </Row>
      </Card>

      {/* Group enable matrix */}
      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div>
          <Title heading={5} style={{ marginTop: 0 }}>
            {t('分组启用矩阵（内容审核）')}
          </Title>
          <Text type='secondary'>
            {t(
              '内容审核默认对所有分组关闭。请按分组启用并选择运行模式（缺省则使用全局模式）。',
            )}
          </Text>
        </div>
        <Table
          style={{ marginTop: 12 }}
          dataSource={riskGroups.items || []}
          rowKey='name'
          size='small'
          pagination={false}
          columns={[
            {
              title: t('分组'),
              dataIndex: 'name',
              render: (v) => <Tag color='cyan'>{v}</Tag>,
            },
            {
              title: t('启用内容审核'),
              dataIndex: 'enabled',
              width: 140,
              render: (_v, record) => (
                <Switch
                  checked={(config.enabled_groups || []).includes(record.name)}
                  onChange={(checked) =>
                    toggleModerationGroup(record.name, checked)
                  }
                />
              ),
            },
            {
              title: t('运行模式'),
              dataIndex: 'mode',
              width: 240,
              render: (_v, record) => {
                const current = (config.group_modes || {})[record.name];
                const value = current === undefined ? '__delete__' : current;
                return (
                  <Select
                    style={{ width: '100%' }}
                    value={value}
                    onChange={(v) => setGroupMode(record.name, v)}
                    getPopupContainer={() => document.body}
                    optionList={[
                      { label: t('未配置（关闭）'), value: '__delete__' },
                      { label: t('跟随全局模式'), value: '' },
                      { label: t('观察模式'), value: 'observe_only' },
                      { label: t('执行模式（预留）'), value: 'enforce' },
                      { label: t('显式关闭'), value: 'off' },
                    ]}
                  />
                );
              },
            },
          ]}
        />
      </Card>

      {/* Moderation rules — AND/OR multi-condition rules over the OpenAI
          response categories. Same edit/save model as 分发检测 rules. */}
      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <div>
            <Title heading={5} style={{ marginTop: 0 }}>
              {t('审核规则')}
            </Title>
            <Text type='secondary'>
              {t(
                '基于 OpenAI 类别评分定义条件组合（AND/OR），命中后写入审核记录。空分组的规则不会生效。',
              )}
            </Text>
          </div>
          <Button
            type='primary'
            onClick={() => {
              setEditingModerationRule(null);
              setRuleEditorVisible(true);
            }}
          >
            {t('新建审核规则')}
          </Button>
        </div>
        <Table
          style={{ marginTop: 12 }}
          dataSource={moderationRules}
          rowKey='id'
          size='small'
          pagination={false}
          scroll={{ x: 'max-content' }}
          empty={
            <Empty
              title={t('暂无规则')}
              description={t('点击"新建审核规则"配置')}
            />
          }
          columns={[
            {
              title: t('启用'),
              dataIndex: 'enabled',
              width: 80,
              render: (v, r) => (
                <Switch
                  checked={v}
                  onChange={(c) => handleToggleModerationRule(r, c)}
                />
              ),
            },
            {
              title: t('名称'),
              dataIndex: 'name',
              render: (_, r) => (
                <Space vertical spacing={2}>
                  <Text strong>{r.name}</Text>
                  <Text type='secondary' size='small'>
                    {r.description || '-'}
                  </Text>
                </Space>
              ),
            },
            {
              title: t('匹配方式'),
              dataIndex: 'match_mode',
              width: 110,
              render: (v) => (
                <Tag color='blue'>
                  {v === 'any' ? t('OR (任一)') : t('AND (全部)')}
                </Tag>
              ),
            },
            {
              title: t('动作'),
              dataIndex: 'action',
              width: 110,
              render: (v) => {
                const colors = { observe: 'orange', flag: 'red', block: 'red' };
                return <Tag color={colors[v] || 'grey'}>{v || 'observe'}</Tag>;
              },
            },
            {
              title: t('适用分组'),
              dataIndex: 'groups',
              render: (v) => {
                const arr = safeParseJSON(v, []);
                if (!arr.length)
                  return <Tag color='red'>{t('未配置（已停用）')}</Tag>;
                return (
                  <Space wrap>
                    {arr.map((g) => (
                      <Tag
                        key={g}
                        color={enabledGroupSet.has(g) ? 'green' : 'grey'}
                      >
                        {g}
                      </Tag>
                    ))}
                  </Space>
                );
              },
            },
            {
              title: t('条件'),
              dataIndex: 'conditions',
              width: 320,
              render: (v) => {
                const arr = safeParseJSON(v, []);
                if (!arr.length) return '-';
                return (
                  <Space vertical spacing={2}>
                    {arr.map((c, i) => (
                      <Text key={i} size='small'>
                        {c.category} {c.op} {c.value}
                        {c.apply_input_type ? ` (${c.applied_input_type})` : ''}
                      </Text>
                    ))}
                  </Space>
                );
              },
            },
            { title: t('优先级'), dataIndex: 'priority', width: 90 },
            {
              title: t('操作'),
              dataIndex: 'operate',
              fixed: 'right',
              width: 160,
              render: (_, r) => (
                <Space>
                  <Button
                    type='tertiary'
                    theme='borderless'
                    onClick={() => {
                      setEditingModerationRule(r);
                      setRuleEditorVisible(true);
                    }}
                  >
                    {t('编辑')}
                  </Button>
                  <Button
                    type='danger'
                    theme='borderless'
                    onClick={() => handleDeleteModerationRule(r)}
                  >
                    {t('删除')}
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <ModerationRuleEditorModal
        visible={ruleEditorVisible}
        loading={savingModerationRule}
        initialValue={editingModerationRule}
        groupOptions={(riskGroups.items || []).map((i) => i.name)}
        enabledGroupSet={enabledGroupSet}
        categories={moderationCategories}
        onCancel={() => {
          setRuleEditorVisible(false);
          setEditingModerationRule(null);
        }}
        onSubmit={handleSaveModerationRule}
      />

      {/* Debug card */}
      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <Title heading={5} style={{ marginTop: 0 }}>
          {t('内容审核调试')}
        </Title>
        <Text type='secondary'>
          {t(
            '管理员手动测试输入文本/图像的审核结果。请求异步入队执行，前端轮询取回结果，超时 30 秒。',
          )}
        </Text>
        <Row gutter={[12, 12]} style={{ marginTop: 12 }}>
          <Col xs={24} md={12}>
            <Text strong>{t('调试分组')}</Text>
            <Select
              style={{ width: '100%' }}
              value={debugGroup}
              onChange={(v) => setDebugGroup(v ?? '')}
              optionList={[
                {
                  label: t('全部规则预览（不限分组）'),
                  value: '',
                },
                ...(riskGroups.items || [])
                  .filter((g) => g.name !== 'auto')
                  .map((g) => ({
                    label: enabledGroupSet.has(g.name)
                      ? g.name
                      : `${g.name} ${t('(分组未启用内容审核)')}`,
                    value: g.name,
                  })),
              ]}
              getPopupContainer={() => document.body}
            />
            <Text
              type='tertiary'
              size='small'
              style={{ display: 'block', marginTop: 6 }}
            >
              {t(
                '选择某个分组将按该分组绑定的规则评估，等同生产流量在该分组下的判定；未选则预览所有启用规则。',
              )}
            </Text>
          </Col>
        </Row>
        <Row gutter={[12, 12]} style={{ marginTop: 12 }}>
          <Col xs={24} md={12}>
            <Text strong>{t('文本输入')}</Text>
            <TextArea
              value={debugText}
              onChange={(v) => setDebugText(v)}
              rows={6}
              placeholder={t('输入需要审核的文本（可留空）')}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text strong>
              {t(
                '图像 URL（每行一个，支持 https:// 或 data:image/...;base64,...）',
              )}
            </Text>
            <TextArea
              value={debugImages}
              onChange={(v) => setDebugImages(v)}
              rows={6}
              placeholder={t('每行一个图像 URL 或 data URI')}
            />
          </Col>
        </Row>
        <div style={{ marginTop: 12 }}>
          <Button type='primary' loading={debugRunning} onClick={runDebug}>
            {t('发起检测')}
          </Button>
        </div>
        {debugError ? (
          <Banner
            type='warning'
            closeIcon={null}
            description={debugError}
            style={{ marginTop: 12 }}
          />
        ) : null}
        {debugResult ? (
          <div style={{ marginTop: 12 }}>
            <Space wrap>
              {/* OpenAI flagged: raw upstream judgment, independent of rules. */}
              <Tag color={debugResult.flagged ? 'red' : 'grey'}>
                {t('OpenAI 原始判定')}:{' '}
                {debugResult.flagged ? t('命中') : t('未命中')}
              </Tag>
              {/* Rule decision: synthesized from the rules bound to the
                  selected debug group (or every enabled rule when group is
                  empty). This is what production traffic would see. */}
              {debugResult.decision ? (
                <Tag
                  color={
                    debugResult.decision.decision === 'block'
                      ? 'red'
                      : debugResult.decision.decision === 'flag'
                        ? 'red'
                        : debugResult.decision.decision === 'observe'
                          ? 'orange'
                          : 'grey'
                  }
                >
                  {t('规则决策')}: {debugResult.decision.decision || 'allow'}
                  {debugResult.decision.primary_rule_name
                    ? ` (${debugResult.decision.primary_rule_name})`
                    : ''}
                </Tag>
              ) : null}
              <Tag color='blue'>
                {t('最高类别')}: {debugResult.max_category || '-'}
              </Tag>
              <Tag color='blue'>
                {t('最高分')}: {(debugResult.max_score ?? 0).toFixed(3)}
              </Tag>
              <Tag color='cyan'>
                {t('上游耗时')}: {debugResult.upstream_latency_ms || 0} ms
              </Tag>
              {debugResult.used_key_suffix ? (
                <Tag>
                  {t('使用 key')}: {debugResult.used_key_suffix}
                </Tag>
              ) : null}
              {debugResult.error ? (
                <Tag color='red'>
                  {t('错误')}: {debugResult.error}
                </Tag>
              ) : null}
            </Space>
            {debugResult.decision &&
            debugResult.decision.matched_rules &&
            debugResult.decision.matched_rules.length > 0 ? (
              <div style={{ marginTop: 8 }}>
                <Text strong>{t('命中规则')}</Text>
                <Space wrap style={{ marginTop: 6 }}>
                  {debugResult.decision.matched_rules.map((r) => (
                    <Tag key={r.rule_id} color='red'>
                      {r.name} ({r.action})
                    </Tag>
                  ))}
                </Space>
              </div>
            ) : null}
            <div style={{ marginTop: 8 }}>
              <Text strong>{t('类别评分')}</Text>
              <Row gutter={[8, 8]} style={{ marginTop: 6 }}>
                {Object.entries(debugResult.categories || {}).map(
                  ([cat, score]) => (
                    <Col key={cat} xs={24} sm={12} md={8} lg={6}>
                      <div
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                        }}
                      >
                        <Text size='small'>{cat}</Text>
                        <Text size='small'>{Number(score).toFixed(3)}</Text>
                      </div>
                      <Progress
                        percent={Math.min(
                          100,
                          Math.round((Number(score) || 0) * 100),
                        )}
                        stroke={
                          score >= 0.7
                            ? 'var(--semi-color-danger)'
                            : score >= 0.3
                              ? 'var(--semi-color-warning)'
                              : 'var(--semi-color-primary)'
                        }
                        showInfo={false}
                      />
                    </Col>
                  ),
                )}
              </Row>
            </div>
          </div>
        ) : null}
      </Card>

      {/* Incidents */}
      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <Title heading={5} style={{ marginTop: 0 }}>
            {t('审核记录')}
          </Title>
          <Space wrap>
            <Select
              value={incidentFilters.flagged}
              style={{ width: 140 }}
              placeholder={t('全部结果')}
              optionList={[
                { label: t('全部结果'), value: '' },
                { label: t('仅命中'), value: 'true' },
                { label: t('仅未命中'), value: 'false' },
              ]}
              onChange={(v) => {
                setIncidentFilters((p) => ({ ...p, flagged: v }));
                loadIncidents(1, incidentsPage.page_size, {
                  ...incidentFilters,
                  flagged: v,
                });
              }}
            />
            <Input
              value={incidentFilters.group}
              placeholder={t('按分组过滤')}
              style={{ width: 160 }}
              onChange={(v) => setIncidentFilters((p) => ({ ...p, group: v }))}
              onEnterPress={() => loadIncidents(1)}
            />
            <Input
              value={incidentFilters.keyword}
              placeholder={t('按用户名/Key/类别')}
              style={{ width: 200 }}
              onChange={(v) =>
                setIncidentFilters((p) => ({ ...p, keyword: v }))
              }
              onEnterPress={() => loadIncidents(1)}
            />
            <Button onClick={() => loadIncidents(1)}>{t('刷新')}</Button>
          </Space>
        </div>
        <Table
          style={{ marginTop: 12 }}
          dataSource={incidents}
          rowKey='id'
          columns={incidentColumns}
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: incidentsPage.page,
            pageSize: incidentsPage.page_size,
            total: incidentsPage.total,
            onPageChange: (page) => loadIncidents(page),
          }}
        />
      </Card>

      <Modal
        title={t('输入内容详情')}
        visible={detailModalVisible}
        onCancel={() => {
          setDetailModalVisible(false);
          setDetailData(null);
        }}
        footer={null}
        centered
        width={700}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{ maxHeight: 'calc(80vh - 120px)', overflowY: 'auto', overflowX: 'hidden' }}
      >
        {detailLoading ? (
          <Spin />
        ) : detailData ? (
          <div>
            <Descriptions row size='small' data={[
              { key: t('请求 ID'), value: detailData.request_id },
              { key: t('用户'), value: detailData.username },
              { key: t('令牌'), value: detailData.token_name },
              { key: t('分组'), value: detailData.group },
              { key: t('决策'), value: detailData.decision },
              { key: t('最高类别'), value: detailData.max_category },
              { key: t('最高分数'), value: typeof detailData.max_score === 'number' ? detailData.max_score.toFixed(4) : '-' },
            ]} />
            <div style={{ marginTop: 16 }}>
              <Text strong>{t('完整输入内容')}</Text>
              <div
                style={{
                  marginTop: 8,
                  padding: 12,
                  background: 'var(--semi-color-fill-0)',
                  borderRadius: 8,
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-all',
                  fontSize: 13,
                  lineHeight: 1.6,
                  maxHeight: 400,
                  overflowY: 'auto',
                  userSelect: 'text',
                }}
              >
                {detailData.input_summary || '-'}
              </div>
            </div>
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

// EnforcementTab is the third top-level tab on /console/risk. It owns the
// unified post-hit policy: which sources participate, whether to email
// users on hit / on auto-ban, the ban threshold + per-source overrides,
// the email rate-limit budget, and surfacing the per-user counter view.
function EnforcementTab() {
  const { t } = useTranslation();
  const [config, setConfig] = useState({
    enabled: false,
    email_on_hit: false,
    email_on_auto_ban: false,
    count_window_hours: 24,
    ban_threshold: 0,
    ban_threshold_per_source: {},
    enabled_sources: ['risk_distribution', 'moderation'],
    email_hit_subject: '',
    email_ban_subject: '',
    email_hit_template: '',
    email_ban_template: '',
    email_rate_limit_window_minutes: 10,
    email_rate_limit_max_per_window: 3,
  });
  const [overview, setOverview] = useState({});
  const [savingConfig, setSavingConfig] = useState(false);
  const [counters, setCounters] = useState([]);
  const [countersPage, setCountersPage] = useState({
    page: 1,
    page_size: 10,
    total: 0,
  });
  const [incidents, setIncidents] = useState([]);
  const [incidentsPage, setIncidentsPage] = useState({
    page: 1,
    page_size: 10,
    total: 0,
  });
  const [incidentFilters, setIncidentFilters] = useState({
    source: '',
    action: '',
    keyword: '',
  });
  const [sendingTest, setSendingTest] = useState(false);

  const loadConfig = async () => {
    const res = await API.get('/api/risk/enforcement/config');
    if (res.data.success)
      setConfig((p) => ({ ...p, ...(res.data.data || {}) }));
  };
  const loadOverview = async () => {
    const res = await API.get('/api/risk/enforcement/overview');
    if (res.data.success) setOverview(res.data.data || {});
  };
  const loadCounters = async (
    page = countersPage.page,
    pageSize = countersPage.page_size,
  ) => {
    const res = await API.get('/api/risk/enforcement/counters', {
      params: { p: page, page_size: pageSize },
    });
    if (res.data.success) {
      const data = res.data.data || {};
      setCounters(data.items || []);
      setCountersPage({
        page: data.page || page,
        page_size: data.page_size || pageSize,
        total: data.total || 0,
      });
    }
  };
  const loadIncidents = async (
    page = incidentsPage.page,
    pageSize = incidentsPage.page_size,
    filters = incidentFilters,
  ) => {
    const params = { p: page, page_size: pageSize };
    if (filters.source) params.source = filters.source;
    if (filters.action) params.action = filters.action;
    if (filters.keyword) params.keyword = filters.keyword;
    const res = await API.get('/api/risk/enforcement/incidents', { params });
    if (res.data.success) {
      const data = res.data.data || {};
      setIncidents(data.items || []);
      setIncidentsPage({
        page: data.page || page,
        page_size: data.page_size || pageSize,
        total: data.total || 0,
      });
    }
  };

  useEffect(() => {
    loadConfig();
    loadOverview();
    loadCounters(1);
    loadIncidents(1);
  }, []);

  const handleSaveConfig = async () => {
    setSavingConfig(true);
    try {
      const res = await API.put('/api/risk/enforcement/config', config);
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('处置策略已保存'));
      setConfig((p) => ({ ...p, ...(res.data.data || {}) }));
      await loadOverview();
    } catch (e) {
      showError(t('保存处置策略失败'));
    } finally {
      setSavingConfig(false);
    }
  };

  const handleResetCounter = async (uid) => {
    Modal.confirm({
      title: t('确认重置该用户的命中计数？'),
      content: t('计数器将清零；账户状态保持不变。'),
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/risk/enforcement/users/${uid}/reset_counter`,
          );
          if (!res.data.success) {
            showError(res.data.message);
            return;
          }
          showSuccess(t('已重置'));
          await Promise.all([loadCounters(), loadIncidents(1)]);
        } catch (e) {
          showError(t('重置失败'));
        }
      },
    });
  };

  const handleUnban = async (uid) => {
    Modal.confirm({
      title: t('确认立即解封该用户？'),
      content: t('账户将恢复启用，且命中计数将清零。'),
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/risk/enforcement/users/${uid}/unban`,
          );
          if (!res.data.success) {
            showError(res.data.message);
            return;
          }
          showSuccess(t('已解封'));
          await Promise.all([loadCounters(), loadIncidents(1), loadOverview()]);
        } catch (e) {
          showError(t('解封失败'));
        }
      },
    });
  };

  const handleSendTestEmail = async () => {
    setSendingTest(true);
    try {
      const res = await API.post('/api/risk/enforcement/test_email');
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      showSuccess(t('测试邮件已发送至当前管理员邮箱'));
    } catch (e) {
      showError(t('发送测试邮件失败'));
    } finally {
      setSendingTest(false);
    }
  };

  const counterColumns = [
    { title: t('用户'), dataIndex: 'username' },
    { title: t('邮箱'), dataIndex: 'email' },
    {
      title: t('账户状态'),
      dataIndex: 'status',
      render: (v) =>
        v === 1 ? (
          <Tag color='green'>{t('正常')}</Tag>
        ) : (
          <Tag color='red'>{t('已禁用')}</Tag>
        ),
    },
    { title: t('分发命中'), dataIndex: 'enforcement_hit_count_risk' },
    { title: t('内容命中'), dataIndex: 'enforcement_hit_count_moderation' },
    {
      title: t('窗口起始'),
      dataIndex: 'enforcement_window_start_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    {
      title: t('最近命中'),
      dataIndex: 'enforcement_last_hit_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    {
      title: t('自动封禁时间'),
      dataIndex: 'enforcement_auto_banned_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      render: (_, r) => (
        <Space>
          <Button
            type='tertiary'
            theme='borderless'
            onClick={() => handleResetCounter(r.id)}
          >
            {t('重置计数')}
          </Button>
          <Button
            type='warning'
            theme='borderless'
            disabled={!r.enforcement_auto_banned_at && r.status === 1}
            onClick={() => handleUnban(r.id)}
          >
            {t('立即解封')}
          </Button>
        </Space>
      ),
    },
  ];

  const incidentColumns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => (v ? timestamp2string(v) : '-'),
    },
    { title: t('用户'), dataIndex: 'username' },
    {
      title: t('分组'),
      dataIndex: 'group',
      render: (v) => <Tag color='cyan'>{v || '-'}</Tag>,
    },
    {
      title: t('来源'),
      dataIndex: 'source',
      render: (v) => {
        const m = {
          risk_distribution: t('分发检测'),
          moderation: t('内容审核'),
          test: t('测试'),
        };
        const c = {
          risk_distribution: 'blue',
          moderation: 'orange',
          test: 'grey',
        };
        return <Tag color={c[v] || 'grey'}>{m[v] || v}</Tag>;
      },
    },
    {
      title: t('动作'),
      dataIndex: 'action',
      render: (v) => {
        const m = {
          hit: t('命中'),
          auto_ban: t('自动封禁'),
          manual_unban: t('解封'),
          counter_reset: t('计数重置'),
          test_email: t('测试邮件'),
        };
        const c = {
          hit: 'orange',
          auto_ban: 'red',
          manual_unban: 'green',
          counter_reset: 'blue',
          test_email: 'grey',
        };
        return <Tag color={c[v] || 'grey'}>{m[v] || v}</Tag>;
      },
    },
    {
      title: t('计数/阈值'),
      dataIndex: 'hit_count_after',
      render: (v, r) => `${v} / ${r.threshold || 0}`,
    },
    {
      title: t('邮件状态'),
      dataIndex: 'email_delivered',
      render: (v, r) =>
        v ? (
          <Tag color='green'>{t('已发送')}</Tag>
        ) : r.email_skip_reason ? (
          <Tag color='grey'>{r.email_skip_reason}</Tag>
        ) : (
          <Tag color='grey'>{t('未发送')}</Tag>
        ),
    },
    {
      title: t('原因'),
      dataIndex: 'reason',
      width: 280,
      render: (v) => <Text ellipsis={{ showTooltip: true }}>{v || '-'}</Text>,
    },
  ];

  return (
    <div className='flex flex-col gap-3'>
      <Banner
        type='info'
        closeIcon={null}
        description={t(
          '处置操作层在分发检测和内容审核命中后统一处理：发送邮件提醒、累计计数、达到阈值自动封禁。邮件复用工单系统通道。',
        )}
      />

      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <Row gutter={[12, 12]}>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('启用状态')}
              value={overview.enabled ? t('已启用') : t('未启用')}
              extra={`${t('窗口')}: ${overview.window_hours ?? 0}h`}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('24h 命中数')}
              value={overview.hits_24h || 0}
              extra={`${t('阈值')}: ${overview.ban_threshold ?? 0}`}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('24h 自动封禁数')}
              value={overview.auto_bans_24h || 0}
              extra={`${t('启用源')}: ${(overview.enabled_sources || []).length}`}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <OverviewCard
              title={t('邮件提醒')}
              value={`${overview.email_on_hit ? t('命中:开') : t('命中:关')} / ${overview.email_on_autoban ? t('封禁:开') : t('封禁:关')}`}
              extra={t('复用工单邮件通道')}
            />
          </Col>
        </Row>
      </Card>

      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <div>
            <Title heading={5} style={{ marginTop: 0 }}>
              {t('全局策略')}
            </Title>
            <Text type='secondary'>
              {t(
                '启用、计数窗口、阈值、邮件开关与模板。模板支持变量：username/time/group/source_zh/count/threshold。',
              )}
            </Text>
          </div>
          <Space>
            <Button loading={sendingTest} onClick={handleSendTestEmail}>
              {t('发送测试邮件至当前管理员')}
            </Button>
            <Button
              type='primary'
              loading={savingConfig}
              onClick={handleSaveConfig}
            >
              {t('保存策略')}
            </Button>
          </Space>
        </div>
        <Row gutter={[12, 12]} style={{ marginTop: 14 }}>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('启用处置层')}</Text>
            <div style={{ marginTop: 10 }}>
              <Switch
                checked={!!config.enabled}
                onChange={(v) => setConfig((p) => ({ ...p, enabled: v }))}
              />
            </div>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('命中时发邮件')}</Text>
            <div style={{ marginTop: 10 }}>
              <Switch
                checked={!!config.email_on_hit}
                onChange={(v) => setConfig((p) => ({ ...p, email_on_hit: v }))}
              />
            </div>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('封禁时发邮件')}</Text>
            <div style={{ marginTop: 10 }}>
              <Switch
                checked={!!config.email_on_auto_ban}
                onChange={(v) =>
                  setConfig((p) => ({ ...p, email_on_auto_ban: v }))
                }
              />
            </div>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('启用源')}</Text>
            <Select
              multiple
              style={{ width: '100%' }}
              value={config.enabled_sources || []}
              onChange={(v) =>
                setConfig((p) => ({ ...p, enabled_sources: v || [] }))
              }
              optionList={[
                { label: t('分发检测'), value: 'risk_distribution' },
                { label: t('内容审核'), value: 'moderation' },
              ]}
              getPopupContainer={() => document.body}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('计数窗口（小时，0=永久累计）')}</Text>
            <InputNumber
              min={0}
              value={config.count_window_hours}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  count_window_hours: typeof v === 'number' ? v : 0,
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('默认封禁阈值（0=不自动封禁）')}</Text>
            <InputNumber
              min={0}
              value={config.ban_threshold}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  ban_threshold: typeof v === 'number' ? v : 0,
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('分发检测专属阈值（0=回退默认）')}</Text>
            <InputNumber
              min={0}
              value={config.ban_threshold_per_source?.risk_distribution ?? 0}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  ban_threshold_per_source: {
                    ...(p.ban_threshold_per_source || {}),
                    risk_distribution: typeof v === 'number' ? v : 0,
                  },
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('内容审核专属阈值（0=回退默认）')}</Text>
            <InputNumber
              min={0}
              value={config.ban_threshold_per_source?.moderation ?? 0}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  ban_threshold_per_source: {
                    ...(p.ban_threshold_per_source || {}),
                    moderation: typeof v === 'number' ? v : 0,
                  },
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('邮件冷却窗口（分钟）')}</Text>
            <InputNumber
              min={1}
              value={config.email_rate_limit_window_minutes}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  email_rate_limit_window_minutes: v || 10,
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Text strong>{t('窗口内最多邮件数')}</Text>
            <InputNumber
              min={1}
              value={config.email_rate_limit_max_per_window}
              onChange={(v) =>
                setConfig((p) => ({
                  ...p,
                  email_rate_limit_max_per_window: v || 3,
                }))
              }
              style={{ width: '100%' }}
            />
          </Col>
          <Col xs={24} md={12}>
            <Text strong>{t('命中邮件主题')}</Text>
            <Input
              value={config.email_hit_subject}
              onChange={(v) =>
                setConfig((p) => ({ ...p, email_hit_subject: v }))
              }
            />
          </Col>
          <Col xs={24} md={12}>
            <Text strong>{t('封禁邮件主题')}</Text>
            <Input
              value={config.email_ban_subject}
              onChange={(v) =>
                setConfig((p) => ({ ...p, email_ban_subject: v }))
              }
            />
          </Col>
          <Col xs={24}>
            <Text strong>{t('命中邮件模板（HTML，支持变量）')}</Text>
            <TextArea
              rows={5}
              value={config.email_hit_template}
              onChange={(v) =>
                setConfig((p) => ({ ...p, email_hit_template: v }))
              }
            />
          </Col>
          <Col xs={24}>
            <Text strong>{t('封禁邮件模板（HTML，支持变量）')}</Text>
            <TextArea
              rows={5}
              value={config.email_ban_template}
              onChange={(v) =>
                setConfig((p) => ({ ...p, email_ban_template: v }))
              }
            />
          </Col>
        </Row>
      </Card>

      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <Title heading={5} style={{ marginTop: 0 }}>
            {t('用户计数器')}
          </Title>
          <Button onClick={() => loadCounters(1)}>{t('刷新')}</Button>
        </div>
        <Table
          style={{ marginTop: 12 }}
          dataSource={counters}
          rowKey='id'
          columns={counterColumns}
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: countersPage.page,
            pageSize: countersPage.page_size,
            total: countersPage.total,
            onPageChange: (page) => loadCounters(page),
          }}
        />
      </Card>

      <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
        <div className='flex items-center justify-between gap-3 flex-wrap'>
          <Title heading={5} style={{ marginTop: 0 }}>
            {t('处置事件流水')}
          </Title>
          <Space wrap>
            <Select
              value={incidentFilters.source}
              style={{ width: 160 }}
              placeholder={t('全部来源')}
              optionList={[
                { label: t('全部来源'), value: '' },
                { label: t('分发检测'), value: 'risk_distribution' },
                { label: t('内容审核'), value: 'moderation' },
                { label: t('测试'), value: 'test' },
              ]}
              onChange={(v) => {
                setIncidentFilters((p) => ({ ...p, source: v }));
                loadIncidents(1, incidentsPage.page_size, {
                  ...incidentFilters,
                  source: v,
                });
              }}
            />
            <Select
              value={incidentFilters.action}
              style={{ width: 160 }}
              placeholder={t('全部动作')}
              optionList={[
                { label: t('全部动作'), value: '' },
                { label: t('命中'), value: 'hit' },
                { label: t('自动封禁'), value: 'auto_ban' },
                { label: t('解封'), value: 'manual_unban' },
                { label: t('计数重置'), value: 'counter_reset' },
                { label: t('测试邮件'), value: 'test_email' },
              ]}
              onChange={(v) => {
                setIncidentFilters((p) => ({ ...p, action: v }));
                loadIncidents(1, incidentsPage.page_size, {
                  ...incidentFilters,
                  action: v,
                });
              }}
            />
            <Input
              value={incidentFilters.keyword}
              placeholder={t('按用户名/原因')}
              style={{ width: 200 }}
              onChange={(v) =>
                setIncidentFilters((p) => ({ ...p, keyword: v }))
              }
              onEnterPress={() => loadIncidents(1)}
            />
            <Button onClick={() => loadIncidents(1)}>{t('刷新')}</Button>
          </Space>
        </div>
        <Table
          style={{ marginTop: 12 }}
          dataSource={incidents}
          rowKey='id'
          columns={incidentColumns}
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: incidentsPage.page,
            pageSize: incidentsPage.page_size,
            total: incidentsPage.total,
            onPageChange: (page) => loadIncidents(page),
          }}
        />
      </Card>
    </div>
  );
}

const RiskCenter = () => {
  const { t } = useTranslation();
  // topTab toggles between the original distribution-detection workflow
  // and the new omni-moderation workflow. Two engines, two tabs — keeps
  // the admin mental model simple and avoids growing the existing tab strip.
  const [topTab, setTopTab] = useState('distribution');

  const [loading, setLoading] = useState(true);
  const [savingConfig, setSavingConfig] = useState(false);
  const [savingRule, setSavingRule] = useState(false);
  const [detectingIP, setDetectingIP] = useState(false);
  const [diagnosisVisible, setDiagnosisVisible] = useState(false);
  const [overview, setOverview] = useState({});
  const [config, setConfig] = useState({});
  const [ipDiagnosis, setIPDiagnosis] = useState(null);
  const [rules, setRules] = useState([]);
  const [subjects, setSubjects] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [subjectsPage, setSubjectsPage] = useState({
    page: 1,
    page_size: 10,
    total: 0,
  });
  const [incidentsPage, setIncidentsPage] = useState({
    page: 1,
    page_size: 10,
    total: 0,
  });
  const [subjectFilters, setSubjectFilters] = useState({
    scope: '',
    status: '',
    keyword: '',
  });
  const [incidentFilters, setIncidentFilters] = useState({
    scope: '',
    action: '',
    keyword: '',
  });
  const [editorVisible, setEditorVisible] = useState(false);
  const [editingRule, setEditingRule] = useState(null);
  // riskGroups holds the GET /api/risk/groups response. Used to render the
  // group enablement matrix and to seed the rule editor's group multi-select.
  const [riskGroups, setRiskGroups] = useState({ items: [] });

  const enabledGroupSet = useMemo(() => {
    const set = new Set();
    for (const item of riskGroups.items || []) {
      if (item.enabled) set.add(item.name);
    }
    return set;
  }, [riskGroups]);

  const groupOptions = useMemo(() => {
    return (riskGroups.items || []).map((item) => item.name);
  }, [riskGroups]);

  const loadOverview = async () => {
    const res = await API.get('/api/risk/overview');
    if (res.data.success) {
      setOverview(res.data.data || {});
      return;
    }
    throw new Error(res.data.message);
  };

  const loadRiskGroups = async () => {
    const res = await API.get('/api/risk/groups');
    if (res.data.success) {
      setRiskGroups(res.data.data || { items: [] });
      return;
    }
    throw new Error(res.data.message);
  };

  const loadConfig = async () => {
    const res = await API.get('/api/risk/config');
    if (res.data.success) {
      setConfig(res.data.data || {});
      return;
    }
    throw new Error(res.data.message);
  };

  const loadRules = async () => {
    const res = await API.get('/api/risk/rules');
    if (res.data.success) {
      setRules(res.data.data || []);
      return;
    }
    throw new Error(res.data.message);
  };

  const loadSubjects = async (
    page = subjectsPage.page,
    pageSize = subjectsPage.page_size,
    filters = subjectFilters,
  ) => {
    const res = await API.get('/api/risk/subjects', {
      params: {
        p: page,
        page_size: pageSize,
        ...filters,
      },
    });
    if (res.data.success) {
      const data = res.data.data || {};
      setSubjects(data.items || []);
      setSubjectsPage({
        page: data.page || page,
        page_size: data.page_size || pageSize,
        total: data.total || 0,
      });
      return;
    }
    throw new Error(res.data.message);
  };

  const loadIncidents = async (
    page = incidentsPage.page,
    pageSize = incidentsPage.page_size,
    filters = incidentFilters,
  ) => {
    const res = await API.get('/api/risk/incidents', {
      params: {
        p: page,
        page_size: pageSize,
        ...filters,
      },
    });
    if (res.data.success) {
      const data = res.data.data || {};
      setIncidents(data.items || []);
      setIncidentsPage({
        page: data.page || page,
        page_size: data.page_size || pageSize,
        total: data.total || 0,
      });
      return;
    }
    throw new Error(res.data.message);
  };

  const refreshAll = async () => {
    try {
      setLoading(true);
      await Promise.all([
        loadOverview(),
        loadConfig(),
        loadRules(),
        loadSubjects(1),
        loadIncidents(1),
        loadRiskGroups(),
      ]);
    } catch (error) {
      showError(error.message || t('加载风控中心失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refreshAll();
  }, []);

  const handleSaveConfig = async () => {
    try {
      setSavingConfig(true);
      const res = await API.put('/api/risk/config', config);
      if (!res.data.success) {
        return showError(res.data.message);
      }
      showSuccess(t('风控配置已保存'));
      setConfig(res.data.data || {});
      await Promise.all([loadOverview(), loadRiskGroups()]);
    } catch (error) {
      showError(t('保存风控配置失败'));
    } finally {
      setSavingConfig(false);
    }
  };

  // toggleGroupEnabled flips the (group, enabled) bit in EnabledGroups and
  // synchronizes config locally; admin still needs to click "保存" to persist.
  const toggleGroupEnabled = (groupName, enabled) => {
    setConfig((prev) => {
      const list = Array.isArray(prev.enabled_groups)
        ? [...prev.enabled_groups]
        : [];
      const next = list.filter((g) => g !== groupName);
      if (enabled) next.push(groupName);
      return { ...prev, enabled_groups: next };
    });
  };

  const setGroupMode = (groupName, mode) => {
    setConfig((prev) => {
      const modes = { ...(prev.group_modes || {}) };
      if (mode === '__delete__') {
        delete modes[groupName];
      } else {
        modes[groupName] = mode;
      }
      return { ...prev, group_modes: modes };
    });
  };

  const handleDetectIP = async () => {
    try {
      setDetectingIP(true);
      setDiagnosisVisible(true);
      const res = await API.get('/api/risk/detect-ip');
      if (!res.data.success) {
        setDiagnosisVisible(false);
        return showError(res.data.message);
      }
      setIPDiagnosis(res.data.data || null);
    } catch (error) {
      setDiagnosisVisible(false);
      showError(t('IP 诊断失败'));
    } finally {
      setDetectingIP(false);
    }
  };

  const handleApplyIPRecommendation = () => {
    if (!ipDiagnosis) {
      return;
    }
    if (
      ipDiagnosis.recommended_mode === 'trusted_header' &&
      ipDiagnosis.recommended_header
    ) {
      setConfig((prev) => ({
        ...prev,
        trusted_ip_header_enabled: true,
        trusted_ip_header: ipDiagnosis.recommended_header,
      }));
    } else {
      setConfig((prev) => ({
        ...prev,
        trusted_ip_header_enabled: false,
      }));
    }
    showSuccess(t('已应用诊断推荐，请保存全局策略'));
  };

  const handleSaveRule = async (form) => {
    try {
      setSavingRule(true);
      const payload = { ...form };
      const res = form.id
        ? await API.put(`/api/risk/rules/${form.id}`, payload)
        : await API.post('/api/risk/rules', payload);
      if (!res.data.success) {
        return showError(res.data.message);
      }
      showSuccess(t('规则已保存'));
      setEditorVisible(false);
      setEditingRule(null);
      await Promise.all([
        loadRules(),
        loadOverview(),
        loadSubjects(1),
        loadIncidents(1),
      ]);
    } catch (error) {
      showError(t('保存规则失败'));
    } finally {
      setSavingRule(false);
    }
  };

  const handleDeleteRule = async (rule) => {
    Modal.confirm({
      title: t('确认删除规则'),
      content: `${t('规则')}: ${rule?.name || '-'}`,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/risk/rules/${rule.id}`);
          if (!res.data.success) {
            return showError(res.data.message);
          }
          showSuccess(t('规则已删除'));
          await Promise.all([loadRules(), loadOverview()]);
        } catch (error) {
          showError(t('删除规则失败'));
        }
      },
    });
  };

  const handleToggleRule = async (rule, enabled) => {
    const groups = safeParseJSON(rule.groups, []);
    if (enabled && (!groups || groups.length === 0)) {
      return showError(t('启用规则前必须至少选择一个分组'));
    }
    await handleSaveRule({
      ...rule,
      conditions: safeParseJSON(rule.conditions, []),
      groups,
      enabled,
    });
  };

  // handleUnblock — v4 sends ?group= because the engine stores blocks under
  // (scope, subjectID, group). Falls back to record.group set on each row.
  const handleUnblock = async (record) => {
    if (!record.group) {
      return showError(t('解封必须指定分组'));
    }
    try {
      const res = await API.post(
        `/api/risk/subjects/${record.subject_type}/${record.subject_id}/unblock`,
        null,
        { params: { group: record.group } },
      );
      if (!res.data.success) {
        return showError(res.data.message);
      }
      showSuccess(t('已解除封禁'));
      await Promise.all([
        loadOverview(),
        loadSubjects(),
        loadIncidents(),
        loadRiskGroups(),
      ]);
    } catch (error) {
      showError(t('解除封禁失败'));
    }
  };

  const subjectColumns = useMemo(
    () => [
      {
        title: t('主体'),
        dataIndex: 'subject_type',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <div>
              <Tag color={record.subject_type === 'token' ? 'blue' : 'green'}>
                {record.subject_type === 'token' ? 'API Key' : t('用户')}
              </Tag>
            </div>
            <Text type='secondary' size='small'>
              #{record.subject_id}
            </Text>
          </Space>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'username',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Text strong>{record.username || '-'}</Text>
            <Text type='secondary' size='small'>
              UID {record.user_id || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('API Key'),
        dataIndex: 'token_name',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Text>{record.token_name || '-'}</Text>
            <Text type='secondary' size='small'>
              {record.token_masked_key || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('分组'),
        dataIndex: 'group',
        render: (value) => <Tag color='cyan'>{value || t('（未知）')}</Tag>,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (value) => renderStatus(value),
      },
      {
        title: t('可疑度'),
        dataIndex: 'risk_score',
        width: 170,
        render: (value) => (
          <Progress
            percent={value || 0}
            showInfo
            stroke={
              value >= 80
                ? 'var(--semi-color-danger)'
                : value >= 50
                  ? 'var(--semi-color-warning)'
                  : 'var(--semi-color-primary)'
            }
          />
        ),
      },
      {
        title: t('10分钟 IP'),
        dataIndex: 'distinct_ip_10m',
      },
      {
        title: t('1小时 IP'),
        dataIndex: 'distinct_ip_1h',
      },
      {
        title: t('1分钟请求'),
        dataIndex: 'request_count_1m',
      },
      {
        title: t('10分钟请求'),
        dataIndex: 'request_count_10m',
      },
      {
        title: t('当前并发'),
        dataIndex: 'inflight_now',
      },
      {
        title: t('命中规则'),
        dataIndex: 'active_rule_names',
        render: (value) => {
          const names = safeParseJSON(value, []);
          if (!names.length) {
            return <Text type='tertiary'>-</Text>;
          }
          return (
            <Space wrap>
              {names.map((item) => (
                <Tag key={item} color='orange'>
                  {item}
                </Tag>
              ))}
            </Space>
          );
        },
      },
      {
        title: t('最后访问'),
        dataIndex: 'last_seen_at',
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('恢复时间'),
        dataIndex: 'recover_at',
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        width: 110,
        render: (_, record) => (
          <Button
            type='primary'
            theme='borderless'
            disabled={record.status !== 'blocked'}
            onClick={() => handleUnblock(record)}
          >
            {t('解除')}
          </Button>
        ),
      },
    ],
    [t],
  );

  const incidentColumns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
      {
        title: t('主体'),
        dataIndex: 'subject_type',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Tag color={record.subject_type === 'token' ? 'blue' : 'green'}>
              {record.subject_type === 'token' ? 'API Key' : t('用户')}
            </Tag>
            <Text type='secondary' size='small'>
              #{record.subject_id}
            </Text>
          </Space>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'username',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Text>{record.username || '-'}</Text>
            <Text type='secondary' size='small'>
              UID {record.user_id || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('API Key'),
        dataIndex: 'token_name',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Text>{record.token_name || '-'}</Text>
            <Text type='secondary' size='small'>
              {record.token_masked_key || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('分组'),
        dataIndex: 'group',
        render: (value) => <Tag color='cyan'>{value || t('（未知）')}</Tag>,
      },
      {
        title: t('规则'),
        dataIndex: 'rule_name',
        render: (value) => value || '-',
      },
      {
        title: t('动作'),
        dataIndex: 'action',
        render: (value) => renderDecision(value),
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (value) => renderStatus(value),
      },
      {
        title: t('可疑度'),
        dataIndex: 'risk_score',
      },
      {
        title: t('路径'),
        dataIndex: 'request_path',
        render: (value) => value || '-',
      },
      {
        title: t('原因'),
        dataIndex: 'reason',
        width: 300,
        render: (value) => (
          <Text ellipsis={{ showTooltip: true }}>{value || '-'}</Text>
        ),
      },
      {
        title: t('恢复时间'),
        dataIndex: 'recover_at',
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
    ],
    [t],
  );

  const ruleColumns = useMemo(
    () => [
      {
        title: t('启用'),
        dataIndex: 'enabled',
        width: 90,
        render: (value, record) => (
          <Switch
            checked={value}
            onChange={(checked) => handleToggleRule(record, checked)}
          />
        ),
      },
      {
        title: t('名称'),
        dataIndex: 'name',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Text strong>{record.name}</Text>
            <Text type='secondary' size='small'>
              {record.description || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('作用域'),
        dataIndex: 'scope',
        render: (value) => (
          <Tag color={value === 'token' ? 'blue' : 'green'}>
            {value === 'token' ? 'API Key' : t('用户')}
          </Tag>
        ),
      },
      {
        title: t('适用分组'),
        dataIndex: 'groups',
        render: (value) => {
          const arr = safeParseJSON(value, []);
          if (!arr.length) {
            return <Tag color='red'>{t('未配置（已停用）')}</Tag>;
          }
          return (
            <Space wrap>
              {arr.map((g) => {
                const listed = enabledGroupSet.has(g);
                return (
                  <Tag key={g} color={listed ? 'green' : 'grey'}>
                    {listed ? g : `${g} ${t('(分组未启用风控)')}`}
                  </Tag>
                );
              })}
            </Space>
          );
        },
      },
      {
        title: t('动作'),
        dataIndex: 'action',
        render: (value, record) => (
          <Space wrap>
            {renderDecision(value)}
            {record.auto_block ? <Tag color='red'>{t('自动封禁')}</Tag> : null}
            {record.auto_recover ? (
              <Tag color='green'>{t('自动恢复')}</Tag>
            ) : null}
          </Space>
        ),
      },
      {
        title: t('条件'),
        dataIndex: 'conditions',
        width: 320,
        render: (value) => {
          const conditions = safeParseJSON(value, []);
          if (!conditions.length) return '-';
          return (
            <Space vertical spacing={2}>
              {conditions.map((condition, index) => (
                <Text key={`${condition.metric}-${index}`} size='small'>
                  {formatRiskCondition(condition)}
                </Text>
              ))}
            </Space>
          );
        },
      },
      {
        title: t('优先级'),
        dataIndex: 'priority',
      },
      {
        title: t('恢复时间'),
        dataIndex: 'recover_after_seconds',
        render: (value, record) =>
          record.auto_recover && record.recover_mode === 'ttl'
            ? `${value || 0}s`
            : t('手动'),
      },
      {
        title: t('返回'),
        dataIndex: 'response_status_code',
        render: (_, record) => (
          <Space vertical spacing={2}>
            <Text>{record.response_status_code || '-'}</Text>
            <Text
              type='secondary'
              size='small'
              ellipsis={{ showTooltip: true }}
            >
              {record.response_message || '-'}
            </Text>
          </Space>
        ),
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        width: 160,
        render: (_, record) => (
          <Space>
            <Button
              type='tertiary'
              theme='borderless'
              onClick={() => {
                setEditingRule(record);
                setEditorVisible(true);
              }}
            >
              {t('编辑')}
            </Button>
            <Button
              type='danger'
              theme='borderless'
              onClick={() => handleDeleteRule(record)}
            >
              {t('删除')}
            </Button>
          </Space>
        ),
      },
    ],
    [t, rules, enabledGroupSet],
  );

  return (
    <div className='mt-[60px] px-2 pb-6'>
      {/* Top-level tab strip: 分发检测（原有风控引擎） / 内容审核（omni-moderation）。
          The two tabs intentionally share the page chrome and the riskGroups
          list; everything else is decoupled. */}
      <Tabs
        type='line'
        activeKey={topTab}
        onChange={setTopTab}
        style={{ marginBottom: 12 }}
      >
        <TabPane tab={t('分发检测')} itemKey='distribution' />
        <TabPane tab={t('内容审核')} itemKey='moderation' />
        <TabPane tab={t('处置操作')} itemKey='enforcement' />
      </Tabs>

      {topTab === 'moderation' ? (
        <ModerationTab riskGroups={riskGroups} />
      ) : null}

      {topTab === 'enforcement' ? <EnforcementTab /> : null}

      {topTab !== 'distribution' ? null : (
        <>
          <RuleEditorModal
            visible={editorVisible}
            loading={savingRule}
            initialValue={editingRule}
            groupOptions={groupOptions}
            enabledGroupSet={enabledGroupSet}
            onCancel={() => {
              setEditorVisible(false);
              setEditingRule(null);
            }}
            onSubmit={handleSaveRule}
          />

          <Spin spinning={loading} size='large'>
            <div className='flex flex-col gap-3'>
              <Banner
                type='warning'
                closeIcon={null}
                description={t(
                  '风控中心采用异步事件引擎。主请求链路只做极轻量封禁态检查，因此不会在首次异常请求上做重计算，后续请求会根据规则命中快速进入封禁或观察。',
                )}
              />

              <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
                <div className='flex flex-col gap-3'>
                  <div className='flex flex-col md:flex-row md:items-end md:justify-between gap-3'>
                    <div>
                      <Title heading={4} style={{ margin: 0 }}>
                        {t('风控中心')}
                      </Title>
                      <Text type='secondary'>
                        {t(
                          '集中管理分发检测、自动封禁、恢复策略和风险主体列表',
                        )}
                      </Text>
                    </div>
                    <Space wrap>
                      <Tag color={config.enabled ? 'green' : 'grey'}>
                        {config.enabled ? t('已启用') : t('已关闭')}
                      </Tag>
                      <Tag color={config.mode === 'enforce' ? 'red' : 'orange'}>
                        {config.mode === 'enforce'
                          ? t('执行模式')
                          : t('观察模式')}
                      </Tag>
                    </Space>
                  </div>

                  <Row gutter={[12, 12]}>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('观察中的主体')}
                        value={overview.observed_subjects || 0}
                        extra={t('当前处于高风险观察态的用户 / API key')}
                      />
                    </Col>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('已封禁主体')}
                        value={overview.blocked_subjects || 0}
                        extra={t('当前已进入自动封禁的用户 / API key')}
                      />
                    </Col>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('高可疑主体')}
                        value={overview.high_risk_subjects || 0}
                        extra={t('可疑度 >= 60 的主体')}
                      />
                    </Col>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('规则数量')}
                        value={overview.rule_count || 0}
                        extra={`${t('事件丢弃')}: ${overview.queue_dropped || 0}`}
                      />
                    </Col>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('启用风控的分组数')}
                        value={overview.enabled_group_count || 0}
                        extra={t('已加入白名单且模式不为 off 的分组个数')}
                      />
                    </Col>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('未配置分组的规则数')}
                        value={overview.unconfigured_rule_count || 0}
                        extra={t('启用但未绑定任何分组，引擎会跳过这些规则')}
                      />
                    </Col>
                    <Col xs={24} sm={12} md={6}>
                      <OverviewCard
                        title={t('启用但分组未启用风控的规则数')}
                        value={overview.group_unlisted_rule_count || 0}
                        extra={t('规则的所有 group 都不在白名单中')}
                      />
                    </Col>
                  </Row>
                </div>
              </Card>

              <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
                <div className='flex items-center justify-between gap-3 flex-wrap'>
                  <div>
                    <Title heading={5} style={{ marginTop: 0 }}>
                      {t('全局策略')}
                    </Title>
                    <Text type='secondary'>
                      {t(
                        '控制风控中心是否开启、运行模式，以及默认封禁返回行为。',
                      )}
                    </Text>
                  </div>
                  <Button
                    type='primary'
                    loading={savingConfig}
                    onClick={handleSaveConfig}
                  >
                    {t('保存全局策略')}
                  </Button>
                </div>

                <Row gutter={[12, 12]} style={{ marginTop: 14 }}>
                  <Col xs={24} sm={12} md={6}>
                    <Text strong>{t('开启风控中心')}</Text>
                    <div style={{ marginTop: 10 }}>
                      <Switch
                        checked={config.enabled}
                        onChange={(value) =>
                          setConfig((prev) => ({ ...prev, enabled: value }))
                        }
                      />
                    </div>
                  </Col>
                  <Col xs={24} sm={12} md={6}>
                    <Text strong>{t('运行模式')}</Text>
                    <Select
                      value={config.mode}
                      onChange={(value) =>
                        setConfig((prev) => ({ ...prev, mode: value }))
                      }
                      optionList={[
                        { label: t('关闭'), value: 'off' },
                        { label: t('观察模式'), value: 'observe_only' },
                        { label: t('执行模式'), value: 'enforce' },
                      ]}
                    />
                  </Col>
                  <Col xs={24} sm={12} md={6}>
                    <Text strong>{t('默认状态码')}</Text>
                    <InputNumber
                      value={config.default_status_code}
                      min={200}
                      max={599}
                      style={{ width: '100%' }}
                      onChange={(value) =>
                        setConfig((prev) => ({
                          ...prev,
                          default_status_code: value || 429,
                        }))
                      }
                    />
                  </Col>
                  <Col xs={24} sm={12} md={6}>
                    <Text strong>{t('默认恢复时间（秒）')}</Text>
                    <InputNumber
                      value={config.default_recover_after_secs}
                      min={60}
                      max={86400 * 7}
                      style={{ width: '100%' }}
                      onChange={(value) =>
                        setConfig((prev) => ({
                          ...prev,
                          default_recover_after_secs: value || 900,
                        }))
                      }
                    />
                  </Col>
                  <Col span={24}>
                    <Text strong>{t('默认返回消息')}</Text>
                    <Input
                      value={config.default_response_message}
                      onChange={(value) =>
                        setConfig((prev) => ({
                          ...prev,
                          default_response_message: value,
                        }))
                      }
                      placeholder={t('当前请求触发风控，请稍后再试')}
                    />
                  </Col>
                </Row>

                <Divider margin='16px' />

                <div className='flex items-center justify-between gap-3 flex-wrap'>
                  <div className='flex items-center gap-3'>
                    <div>
                      <Text strong>{t('信任上游 IP 头')}</Text>
                      <Text
                        type='tertiary'
                        size='small'
                        style={{ display: 'block', marginTop: 2 }}
                      >
                        {t(
                          '开启后会统一影响限流、邮箱验证码限流、Turnstile、令牌 IP 白名单、日志记录和风控事件。',
                        )}
                      </Text>
                    </div>
                  </div>
                  <Switch
                    checked={config.trusted_ip_header_enabled}
                    onChange={(value) =>
                      setConfig((prev) => ({
                        ...prev,
                        trusted_ip_header_enabled: value,
                      }))
                    }
                  />
                </div>

                <Banner
                  type='info'
                  closeIcon={null}
                  style={{ marginTop: 12 }}
                  description={
                    config.trusted_ip_header_enabled
                      ? t(
                          '当前已开启“信任上游 IP 头”。系统会优先读取你填写的请求头作为真实 IP，并统一应用到限流、邮箱验证码限流、Turnstile、令牌 IP 白名单、日志记录和风控事件。',
                        )
                      : t(
                          '当前未开启“信任上游 IP 头”。系统会统一使用 TCP RemoteAddr 作为真实 IP，并统一应用到限流、邮箱验证码限流、Turnstile、令牌 IP 白名单、日志记录和风控事件。',
                        )
                  }
                />

                <div style={{ marginTop: 12 }}>
                  {config.trusted_ip_header_enabled && (
                    <Banner
                      type='warning'
                      closeIcon={null}
                      style={{ marginBottom: 12 }}
                      description={t(
                        '请确保反向代理已正确配置该请求头（如 Nginx 的 proxy_set_header），否则所有依赖 IP 的功能将受影响。',
                      )}
                    />
                  )}

                  <Text strong>{t('请求头名称')}</Text>
                  <Input
                    value={config.trusted_ip_header}
                    onChange={(value) =>
                      setConfig((prev) => ({
                        ...prev,
                        trusted_ip_header: value,
                      }))
                    }
                    disabled={!config.trusted_ip_header_enabled}
                    placeholder={t('例如 X-Real-IP 或 CF-Connecting-IP')}
                    style={{ marginTop: 4 }}
                  />
                  <Text
                    type='tertiary'
                    size='small'
                    style={{ display: 'block', marginTop: 6 }}
                  >
                    {t(
                      '只有在你的服务前面有你完全信任的反向代理、Ingress 或 CDN，并且它会覆盖写入真实客户端 IP 请求头时，才应开启。直连公网通常保持关闭；Nginx / Ingress 常用 X-Real-IP；Cloudflare 常用 CF-Connecting-IP。',
                    )}
                  </Text>
                </div>

                <div style={{ marginTop: 8 }}>
                  <Button loading={detectingIP} onClick={handleDetectIP}>
                    {t('检测当前环境')}
                  </Button>
                  <Text type='tertiary' size='small' style={{ marginLeft: 8 }}>
                    {t('不确定填什么？点击检测，系统会自动分析并推荐')}
                  </Text>
                </div>

                <Modal
                  title={t('IP 环境诊断')}
                  visible={diagnosisVisible}
                  onCancel={() => setDiagnosisVisible(false)}
                  footer={
                    <div className='flex justify-between'>
                      <Button onClick={() => setDiagnosisVisible(false)}>
                        {t('关闭')}
                      </Button>
                      <Button
                        type='primary'
                        onClick={() => {
                          handleApplyIPRecommendation();
                          setDiagnosisVisible(false);
                        }}
                      >
                        {t('应用推荐配置')}
                      </Button>
                    </div>
                  }
                  width={720}
                  bodyStyle={{ maxHeight: '65vh', overflowY: 'auto' }}
                >
                  {detectingIP ? (
                    <div style={{ textAlign: 'center', padding: 40 }}>
                      <Spin size='large' />
                    </div>
                  ) : ipDiagnosis ? (
                    <div className='flex flex-col gap-4'>
                      <Banner
                        type={
                          ipDiagnosis.recommended_mode === 'trusted_header'
                            ? 'info'
                            : 'success'
                        }
                        closeIcon={null}
                        description={ipDiagnosis.recommendation_message || '-'}
                      />

                      <Row gutter={[12, 12]}>
                        <Col span={8}>
                          <Text type='secondary' size='small'>
                            {t('当前配置')}
                          </Text>
                          <div style={{ marginTop: 4 }}>
                            <Tag
                              color={
                                ipDiagnosis.current_mode === 'trusted_header'
                                  ? 'orange'
                                  : 'blue'
                              }
                              size='large'
                            >
                              {ipDiagnosis.current_mode === 'trusted_header'
                                ? t('信任请求头')
                                : 'RemoteAddr'}
                            </Tag>
                          </div>
                        </Col>
                        <Col span={8}>
                          <Text type='secondary' size='small'>
                            {t('推荐配置')}
                          </Text>
                          <div style={{ marginTop: 4 }}>
                            <Tag
                              color={
                                ipDiagnosis.recommended_mode ===
                                'trusted_header'
                                  ? 'green'
                                  : 'blue'
                              }
                              size='large'
                            >
                              {ipDiagnosis.recommended_mode === 'trusted_header'
                                ? ipDiagnosis.recommended_header
                                : 'RemoteAddr'}
                            </Tag>
                          </div>
                        </Col>
                        <Col span={8}>
                          <Text type='secondary' size='small'>
                            {t('当前生效 IP')}
                          </Text>
                          <div style={{ marginTop: 4 }}>
                            <Text
                              strong
                              style={{
                                fontFamily:
                                  'ui-monospace, SFMono-Regular, Menlo, monospace',
                              }}
                            >
                              {ipDiagnosis.effective_client_ip || '-'}
                            </Text>
                          </div>
                        </Col>
                      </Row>

                      <Divider margin='4px' />

                      <div>
                        <Text
                          strong
                          style={{ marginBottom: 8, display: 'block' }}
                        >
                          {t('请求头明细')}
                        </Text>
                        <Table
                          dataSource={ipDiagnosis.items || []}
                          rowKey={(record, index) => `${record.name}-${index}`}
                          pagination={false}
                          size='small'
                          columns={[
                            {
                              title: t('来源'),
                              dataIndex: 'name',
                              width: 180,
                              render: (_, record) => (
                                <Space wrap>
                                  <Text strong>{record.name}</Text>
                                  {record.is_current ? (
                                    <Tag color='cyan' size='small'>
                                      {t('生效')}
                                    </Tag>
                                  ) : null}
                                  {ipDiagnosis.recommended_mode ===
                                    'trusted_header' &&
                                  ipDiagnosis.recommended_header ===
                                    record.name ? (
                                    <Tag color='green' size='small'>
                                      {t('推荐')}
                                    </Tag>
                                  ) : null}
                                </Space>
                              ),
                            },
                            {
                              title: t('原始值'),
                              dataIndex: 'raw_value',
                              render: (value) => (
                                <Text
                                  style={{
                                    fontFamily:
                                      'ui-monospace, SFMono-Regular, Menlo, monospace',
                                    wordBreak: 'break-all',
                                    fontSize: 13,
                                  }}
                                >
                                  {value || '-'}
                                </Text>
                              ),
                            },
                            {
                              title: t('解析 IP'),
                              dataIndex: 'parsed_ip',
                              width: 140,
                              render: (value) => (
                                <Text
                                  style={{
                                    fontFamily:
                                      'ui-monospace, SFMono-Regular, Menlo, monospace',
                                    fontSize: 13,
                                  }}
                                >
                                  {value || '-'}
                                </Text>
                              ),
                            },
                            {
                              title: t('类型'),
                              dataIndex: 'classification',
                              width: 90,
                              render: (value) => {
                                const map = {
                                  public: {
                                    color: 'green',
                                    label: t('公网'),
                                  },
                                  private: {
                                    color: 'orange',
                                    label: t('内网'),
                                  },
                                  invalid: {
                                    color: 'red',
                                    label: t('无效'),
                                  },
                                };
                                const info = map[value] || {
                                  color: 'grey',
                                  label: t('无值'),
                                };
                                return (
                                  <Tag color={info.color}>{info.label}</Tag>
                                );
                              },
                            },
                          ]}
                        />
                      </div>

                      <Text type='tertiary' size='small'>
                        {t(
                          '点击”应用推荐配置”会自动填充表单，还需要点击”保存全局策略”才会真正生效。',
                        )}
                      </Text>
                    </div>
                  ) : null}
                </Modal>
              </Card>

              {/* 分组启用矩阵 — v4: per-group whitelist + mode override.
              `auto` is filtered out by the backend (and documented in DEV_GUIDE §11). */}
              <Card bodyStyle={{ padding: 20 }} style={{ borderRadius: 16 }}>
                <div className='flex items-center justify-between gap-3 flex-wrap'>
                  <div>
                    <Title heading={5} style={{ marginTop: 0 }}>
                      {t('分组启用矩阵')}
                    </Title>
                    <Text type='secondary'>
                      {t(
                        '风控分发检测默认对所有分组关闭。请按分组启用风控并选择该分组的运行模式（缺省则使用全局模式）。',
                      )}
                    </Text>
                  </div>
                  <Button
                    type='primary'
                    loading={savingConfig}
                    onClick={handleSaveConfig}
                  >
                    {t('保存全局策略')}
                  </Button>
                </div>
                <Table
                  style={{ marginTop: 12 }}
                  dataSource={riskGroups.items || []}
                  rowKey='name'
                  size='small'
                  pagination={false}
                  columns={[
                    {
                      title: t('分组'),
                      dataIndex: 'name',
                      render: (v) => <Tag color='cyan'>{v}</Tag>,
                    },
                    {
                      title: t('启用风控'),
                      dataIndex: 'enabled',
                      width: 120,
                      render: (_v, record) => (
                        <Switch
                          checked={(config.enabled_groups || []).includes(
                            record.name,
                          )}
                          onChange={(checked) =>
                            toggleGroupEnabled(record.name, checked)
                          }
                        />
                      ),
                    },
                    {
                      title: t('运行模式'),
                      dataIndex: 'mode',
                      width: 200,
                      render: (_v, record) => {
                        const current = (config.group_modes || {})[record.name];
                        const value =
                          current === undefined ? '__delete__' : current;
                        return (
                          <Select
                            style={{ width: '100%' }}
                            value={value}
                            onChange={(v) => setGroupMode(record.name, v)}
                            getPopupContainer={() => document.body}
                            optionList={[
                              {
                                label: t('未配置（关闭）'),
                                value: '__delete__',
                              },
                              { label: t('跟随全局模式'), value: '' },
                              { label: t('观察模式'), value: 'observe_only' },
                              { label: t('执行模式'), value: 'enforce' },
                              { label: t('显式关闭'), value: 'off' },
                            ]}
                          />
                        );
                      },
                    },
                    {
                      title: t('实际生效模式'),
                      dataIndex: 'effective_mode',
                      render: (v) => (
                        <Tag
                          color={
                            v === 'enforce'
                              ? 'red'
                              : v === 'observe_only'
                                ? 'orange'
                                : 'grey'
                          }
                        >
                          {v || 'off'}
                        </Tag>
                      ),
                    },
                    {
                      title: t('规则数（启用/全部）'),
                      dataIndex: 'rule_count_total',
                      render: (_v, r) =>
                        `${r.rule_count_enabled || 0} / ${r.rule_count_total || 0}`,
                    },
                    { title: t('观察主体'), dataIndex: 'active_subject_count' },
                    {
                      title: t('封禁主体'),
                      dataIndex: 'blocked_subject_count',
                    },
                    {
                      title: t('高风险'),
                      dataIndex: 'high_risk_subject_count',
                    },
                  ]}
                />
              </Card>

              <Card
                bodyStyle={{ padding: 0, overflow: 'hidden' }}
                style={{ borderRadius: 16 }}
              >
                <Tabs type='card' collapsible>
                  <TabPane tab={t('风险主体')} itemKey='subjects'>
                    <div style={{ padding: 20 }}>
                      <div className='flex flex-col md:flex-row gap-3 md:items-center md:justify-between mb-4'>
                        <Space wrap>
                          <Select
                            value={subjectFilters.scope}
                            style={{ width: 140 }}
                            placeholder={t('全部作用域')}
                            optionList={[
                              { label: t('全部作用域'), value: '' },
                              { label: 'API Key', value: 'token' },
                              { label: t('用户'), value: 'user' },
                            ]}
                            onChange={(value) =>
                              setSubjectFilters((prev) => ({
                                ...prev,
                                scope: value,
                              }))
                            }
                          />
                          <Select
                            value={subjectFilters.status}
                            style={{ width: 140 }}
                            placeholder={t('全部状态')}
                            optionList={[
                              { label: t('全部状态'), value: '' },
                              { label: t('正常'), value: 'normal' },
                              { label: t('观察中'), value: 'observe' },
                              { label: t('已封禁'), value: 'blocked' },
                            ]}
                            onChange={(value) =>
                              setSubjectFilters((prev) => ({
                                ...prev,
                                status: value,
                              }))
                            }
                          />
                          <Input
                            style={{ width: 260 }}
                            value={subjectFilters.keyword}
                            onChange={(value) =>
                              setSubjectFilters((prev) => ({
                                ...prev,
                                keyword: value,
                              }))
                            }
                            placeholder={t('搜索用户、API key、规则名')}
                          />
                          <Button
                            onClick={() =>
                              loadSubjects(
                                1,
                                subjectsPage.page_size,
                                subjectFilters,
                              )
                            }
                          >
                            {t('查询')}
                          </Button>
                        </Space>
                        <Button
                          onClick={() =>
                            loadSubjects(
                              subjectsPage.page,
                              subjectsPage.page_size,
                              subjectFilters,
                            )
                          }
                        >
                          {t('刷新')}
                        </Button>
                      </div>

                      <Table
                        columns={subjectColumns}
                        dataSource={subjects}
                        rowKey='id'
                        pagination={{
                          currentPage: subjectsPage.page,
                          pageSize: subjectsPage.page_size,
                          total: subjectsPage.total,
                          pageSizeOpts: [10, 20, 50, 100],
                          showSizeChanger: true,
                          onPageChange: (page) =>
                            loadSubjects(
                              page,
                              subjectsPage.page_size,
                              subjectFilters,
                            ),
                          onPageSizeChange: (pageSize) =>
                            loadSubjects(1, pageSize, subjectFilters),
                        }}
                        scroll={{ x: 'max-content' }}
                        empty={
                          <Empty
                            title={t('暂无风险主体')}
                            description={t('当前没有满足条件的风险主体记录')}
                          />
                        }
                      />
                    </div>
                  </TabPane>

                  <TabPane tab={t('命中事件')} itemKey='incidents'>
                    <div style={{ padding: 20 }}>
                      <div className='flex flex-col md:flex-row gap-3 md:items-center md:justify-between mb-4'>
                        <Space wrap>
                          <Select
                            value={incidentFilters.scope}
                            style={{ width: 140 }}
                            placeholder={t('全部作用域')}
                            optionList={[
                              { label: t('全部作用域'), value: '' },
                              { label: 'API Key', value: 'token' },
                              { label: t('用户'), value: 'user' },
                            ]}
                            onChange={(value) =>
                              setIncidentFilters((prev) => ({
                                ...prev,
                                scope: value,
                              }))
                            }
                          />
                          <Select
                            value={incidentFilters.action}
                            style={{ width: 140 }}
                            placeholder={t('全部动作')}
                            optionList={[
                              { label: t('全部动作'), value: '' },
                              { label: t('观察'), value: 'observe' },
                              { label: t('封禁'), value: 'block' },
                              { label: t('恢复'), value: 'recover' },
                              { label: t('手动解除'), value: 'manual_unblock' },
                            ]}
                            onChange={(value) =>
                              setIncidentFilters((prev) => ({
                                ...prev,
                                action: value,
                              }))
                            }
                          />
                          <Input
                            style={{ width: 260 }}
                            value={incidentFilters.keyword}
                            onChange={(value) =>
                              setIncidentFilters((prev) => ({
                                ...prev,
                                keyword: value,
                              }))
                            }
                            placeholder={t('搜索用户、API key、规则、原因')}
                          />
                          <Button
                            onClick={() =>
                              loadIncidents(
                                1,
                                incidentsPage.page_size,
                                incidentFilters,
                              )
                            }
                          >
                            {t('查询')}
                          </Button>
                        </Space>
                        <Button
                          onClick={() =>
                            loadIncidents(
                              incidentsPage.page,
                              incidentsPage.page_size,
                              incidentFilters,
                            )
                          }
                        >
                          {t('刷新')}
                        </Button>
                      </div>

                      <Table
                        columns={incidentColumns}
                        dataSource={incidents}
                        rowKey='id'
                        pagination={{
                          currentPage: incidentsPage.page,
                          pageSize: incidentsPage.page_size,
                          total: incidentsPage.total,
                          pageSizeOpts: [10, 20, 50, 100],
                          showSizeChanger: true,
                          onPageChange: (page) =>
                            loadIncidents(
                              page,
                              incidentsPage.page_size,
                              incidentFilters,
                            ),
                          onPageSizeChange: (pageSize) =>
                            loadIncidents(1, pageSize, incidentFilters),
                        }}
                        scroll={{ x: 'max-content' }}
                        empty={
                          <Empty
                            title={t('暂无命中事件')}
                            description={t('当前没有满足条件的风控事件')}
                          />
                        }
                      />
                    </div>
                  </TabPane>

                  <TabPane tab={t('规则管理')} itemKey='rules'>
                    <div style={{ padding: 20 }}>
                      <div className='flex flex-col md:flex-row gap-3 md:items-center md:justify-between mb-4'>
                        <div>
                          <Title heading={5} style={{ marginTop: 0 }}>
                            {t('规则列表')}
                          </Title>
                          <Text type='secondary'>
                            {t(
                              '每条规则可单独设置作用域、动作、状态码、消息与恢复策略。',
                            )}
                          </Text>
                        </div>
                        <Space>
                          <Button onClick={() => loadRules()}>
                            {t('刷新')}
                          </Button>
                          <Button
                            type='primary'
                            onClick={() => {
                              setEditingRule(null);
                              setEditorVisible(true);
                            }}
                          >
                            {t('新建规则')}
                          </Button>
                        </Space>
                      </div>

                      <Table
                        columns={ruleColumns}
                        dataSource={rules}
                        rowKey='id'
                        pagination={false}
                        scroll={{ x: 'max-content' }}
                        empty={
                          <Empty
                            title={t('暂无规则')}
                            description={t('可以先创建默认的分发检测规则')}
                          />
                        }
                      />
                    </div>
                  </TabPane>
                </Tabs>
              </Card>
            </div>
          </Spin>
        </>
      )}
    </div>
  );
};

export default RiskCenter;

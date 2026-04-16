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
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const METRIC_OPTIONS = [
  { label: '10分钟不同 IP', value: 'distinct_ip_10m' },
  { label: '1小时不同 IP', value: 'distinct_ip_1h' },
  { label: '10分钟不同 UA', value: 'distinct_ua_10m' },
  { label: '1分钟请求数', value: 'request_count_1m' },
  { label: '10分钟请求数', value: 'request_count_10m' },
  { label: '当前并发', value: 'inflight_now' },
  { label: '24小时命中次数', value: 'rule_hit_count_24h' },
  { label: '可疑度', value: 'risk_score' },
];

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

const emptyRuleForm = () => ({
  id: 0,
  name: '',
  description: '',
  enabled: true,
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
  onCancel,
  onSubmit,
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState(emptyRuleForm());

  useEffect(() => {
    if (!visible) return;
    if (initialValue) {
      setForm({
        ...emptyRuleForm(),
        ...initialValue,
        conditions: safeParseJSON(initialValue.conditions, [
          { metric: 'distinct_ip_10m', op: '>=', value: 3 },
        ]),
      });
      return;
    }
    setForm(emptyRuleForm());
  }, [visible, initialValue]);

  const updateField = (field, value) => {
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
        { metric: 'distinct_ip_10m', op: '>=', value: 1 },
      ],
    }));
  };

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
    onSubmit(form);
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
      width={980}
      bodyStyle={{ maxHeight: '72vh', overflowY: 'auto' }}
    >
      <Space vertical align='start' style={{ width: '100%' }} spacing='tight'>
        <Banner
          type='info'
          closeIcon={null}
          description={t(
            '规则采用结构化条件配置。稳定优先，不支持自由脚本；命中后可选择观察或封禁，并自定义返回状态码与恢复策略。',
          )}
        />
        <Row gutter={12} style={{ width: '100%' }}>
          <Col span={8}>
            <Text strong>{t('规则名称')}</Text>
            <Input
              value={form.name}
              onChange={(value) => updateField('name', value)}
              placeholder={t('例如 token_multi_ip_block')}
            />
          </Col>
          <Col span={8}>
            <Text strong>{t('作用域')}</Text>
            <Select
              value={form.scope}
              onChange={(value) => updateField('scope', value)}
              optionList={[
                { label: t('API Key'), value: 'token' },
                { label: t('用户'), value: 'user' },
              ]}
            />
          </Col>
          <Col span={8}>
            <Text strong>{t('检测器')}</Text>
            <Select
              value={form.detector}
              onChange={(value) => updateField('detector', value)}
              optionList={[{ label: t('分发检测'), value: 'distribution' }]}
            />
          </Col>
        </Row>

        <div style={{ width: '100%' }}>
          <Text strong>{t('规则描述')}</Text>
          <TextArea
            value={form.description}
            rows={2}
            maxCount={200}
            onChange={(value) => updateField('description', value)}
            placeholder={t('简要说明此规则的风险目标和作用')}
          />
        </div>

        <Row gutter={12} style={{ width: '100%' }}>
          <Col span={6}>
            <Text strong>{t('匹配方式')}</Text>
            <Select
              value={form.match_mode}
              onChange={(value) => updateField('match_mode', value)}
              optionList={[
                { label: t('全部满足'), value: 'all' },
                { label: t('任一满足'), value: 'any' },
              ]}
            />
          </Col>
          <Col span={6}>
            <Text strong>{t('动作')}</Text>
            <Select
              value={form.action}
              onChange={(value) => updateField('action', value)}
              optionList={[
                { label: t('观察'), value: 'observe' },
                { label: t('封禁'), value: 'block' },
              ]}
            />
          </Col>
          <Col span={6}>
            <Text strong>{t('优先级')}</Text>
            <InputNumber
              value={form.priority}
              min={0}
              max={999}
              style={{ width: '100%' }}
              onChange={(value) => updateField('priority', value || 0)}
            />
          </Col>
          <Col span={6}>
            <Text strong>{t('可疑度权重')}</Text>
            <InputNumber
              value={form.score_weight}
              min={0}
              max={100}
              style={{ width: '100%' }}
              onChange={(value) => updateField('score_weight', value || 0)}
            />
          </Col>
        </Row>

        <Row gutter={12} style={{ width: '100%' }}>
          <Col span={6}>
            <Text strong>{t('规则启用')}</Text>
            <div style={{ paddingTop: 8 }}>
              <Switch
                checked={form.enabled}
                onChange={(value) => updateField('enabled', value)}
              />
            </div>
          </Col>
          <Col span={6}>
            <Text strong>{t('自动封禁')}</Text>
            <div style={{ paddingTop: 8 }}>
              <Switch
                checked={form.auto_block}
                onChange={(value) => updateField('auto_block', value)}
                disabled={form.action !== 'block'}
              />
            </div>
          </Col>
          <Col span={6}>
            <Text strong>{t('自动恢复')}</Text>
            <div style={{ paddingTop: 8 }}>
              <Switch
                checked={form.auto_recover}
                onChange={(value) => updateField('auto_recover', value)}
              />
            </div>
          </Col>
          <Col span={6}>
            <Text strong>{t('恢复方式')}</Text>
            <Select
              value={form.recover_mode}
              onChange={(value) => updateField('recover_mode', value)}
              optionList={[
                { label: t('TTL 自动恢复'), value: 'ttl' },
                { label: t('人工恢复'), value: 'manual' },
              ]}
            />
          </Col>
        </Row>

        <Row gutter={12} style={{ width: '100%' }}>
          <Col span={8}>
            <Text strong>{t('恢复时间（秒）')}</Text>
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
          <Col span={8}>
            <Text strong>{t('返回状态码')}</Text>
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
          <Col span={8}>
            <Text strong>{t('返回消息')}</Text>
            <Input
              value={form.response_message}
              onChange={(value) => updateField('response_message', value)}
              placeholder={t('当前请求触发风控，请稍后再试')}
            />
          </Col>
        </Row>

        <Divider margin='12px' />

        <div style={{ width: '100%' }}>
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
          <Space vertical style={{ width: '100%' }}>
            {form.conditions.map((condition, index) => (
              <Card
                key={`condition-${index}`}
                bodyStyle={{ padding: 14 }}
                style={{
                  width: '100%',
                  borderRadius: 12,
                  border: '1px solid var(--semi-color-border)',
                }}
              >
                <Row gutter={12}>
                  <Col span={9}>
                    <Text strong>{t('指标')}</Text>
                    <Select
                      value={condition.metric}
                      optionList={METRIC_OPTIONS}
                      onChange={(value) =>
                        updateCondition(index, 'metric', value)
                      }
                    />
                  </Col>
                  <Col span={5}>
                    <Text strong>{t('运算')}</Text>
                    <Select
                      value={condition.op}
                      optionList={OP_OPTIONS}
                      onChange={(value) => updateCondition(index, 'op', value)}
                    />
                  </Col>
                  <Col span={6}>
                    <Text strong>{t('阈值')}</Text>
                    <InputNumber
                      value={condition.value}
                      style={{ width: '100%' }}
                      onChange={(value) =>
                        updateCondition(index, 'value', value || 0)
                      }
                    />
                  </Col>
                  <Col span={4}>
                    <Text strong>{t('操作')}</Text>
                    <div style={{ paddingTop: 8 }}>
                      <Button
                        type='danger'
                        theme='borderless'
                        onClick={() => removeCondition(index)}
                        disabled={form.conditions.length === 1}
                      >
                        {t('删除')}
                      </Button>
                    </div>
                  </Col>
                </Row>
              </Card>
            ))}
          </Space>
        </div>
      </Space>
    </Modal>
  );
}

const RiskCenter = () => {
  const { t } = useTranslation();

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

  const loadOverview = async () => {
    const res = await API.get('/api/risk/overview');
    if (res.data.success) {
      setOverview(res.data.data || {});
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
      await loadOverview();
    } catch (error) {
      showError(t('保存风控配置失败'));
    } finally {
      setSavingConfig(false);
    }
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
    await handleSaveRule({
      ...rule,
      conditions: safeParseJSON(rule.conditions, []),
      enabled,
    });
  };

  const handleUnblock = async (record) => {
    try {
      const res = await API.post(
        `/api/risk/subjects/${record.subject_type}/${record.subject_id}/unblock`,
      );
      if (!res.data.success) {
        return showError(res.data.message);
      }
      showSuccess(t('已解除封禁'));
      await Promise.all([loadOverview(), loadSubjects(), loadIncidents()]);
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
    [t, rules],
  );

  return (
    <div className='mt-[60px] px-2 pb-6'>
      <RuleEditorModal
        visible={editorVisible}
        loading={savingRule}
        initialValue={editingRule}
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
                    {t('集中管理分发检测、自动封禁、恢复策略和风险主体列表')}
                  </Text>
                </div>
                <Space wrap>
                  <Tag color={config.enabled ? 'green' : 'grey'}>
                    {config.enabled ? t('已启用') : t('已关闭')}
                  </Tag>
                  <Tag color={config.mode === 'enforce' ? 'red' : 'orange'}>
                    {config.mode === 'enforce' ? t('执行模式') : t('观察模式')}
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
                  {t('控制风控中心是否开启、运行模式，以及默认封禁返回行为。')}
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
              <Button
                loading={detectingIP}
                onClick={handleDetectIP}
              >
                {t('检测当前环境')}
              </Button>
              <Text
                type='tertiary'
                size='small'
                style={{ marginLeft: 8 }}
              >
                {t('不确定填什么？点击检测，系统会自动分析并推荐')}
              </Text>
            </div>

            <Modal
              title={t('IP 环境诊断')}
              visible={diagnosisVisible}
              onCancel={() => setDiagnosisVisible(false)}
              footer={
                <div className='flex justify-between'>
                  <Button
                    onClick={() => setDiagnosisVisible(false)}
                  >
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
                    description={
                      ipDiagnosis.recommendation_message || '-'
                    }
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
                            ipDiagnosis.recommended_mode === 'trusted_header'
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
                    <Text strong style={{ marginBottom: 8, display: 'block' }}>
                      {t('请求头明细')}
                    </Text>
                    <Table
                      dataSource={ipDiagnosis.items || []}
                      rowKey={(record, index) =>
                        `${record.name}-${index}`
                      }
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
                              <Tag color={info.color}>
                                {info.label}
                              </Tag>
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
                      <Button onClick={() => loadRules()}>{t('刷新')}</Button>
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
    </div>
  );
};

export default RiskCenter;

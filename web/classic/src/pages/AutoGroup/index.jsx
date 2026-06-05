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

import React, { useEffect, useState, useCallback } from 'react';
import {
  Table,
  Button,
  Modal,
  Select,
  Switch,
  Tag,
  Tabs,
  TabPane,
  InputNumber,
  Input,
  Space,
  SideSheet,
  Empty,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const METRICS = [
  { key: 'total_spend', label: '累计消费 (USD)', needsParam: false },
  { key: 'recent_spend', label: '近 N 小时消费 (USD)', needsParam: true },
  { key: 'yesterday_spend', label: '昨日消费 (USD)', needsParam: false },
  { key: 'total_topup', label: '净充值额 (USD)', needsParam: false },
  { key: 'recent_topup', label: '近 N 小时充值 (USD)', needsParam: true },
  { key: 'yesterday_topup', label: '昨日充值 (USD)', needsParam: false },
  { key: 'total_request_count', label: '累计请求数', needsParam: false },
  { key: 'recent_request_count', label: '近 N 小时请求数', needsParam: true },
  { key: 'current_group', label: '当前分组', needsParam: false },
];

const METRIC_OPTIONS = METRICS.map((m) => ({ label: m.label, value: m.key }));
const METRIC_MAP = Object.fromEntries(METRICS.map((m) => [m.key, m]));

const STRING_METRICS = new Set(['current_group']);
const STRING_OPS = [
  { label: '==', value: '==' },
  { label: '!=', value: '!=' },
];

const OP_OPTIONS = [
  { label: '>=', value: '>=' },
  { label: '>', value: '>' },
  { label: '<=', value: '<=' },
  { label: '<', value: '<' },
  { label: '==', value: '==' },
  { label: '!=', value: '!=' },
];

const PAGE_SIZE = 10;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function safeParseJSON(value, fallback = []) {
  if (!value) return fallback;
  if (Array.isArray(value)) return value;
  try {
    return JSON.parse(value);
  } catch {
    return fallback;
  }
}

function emptyRuleForm() {
  return {
    id: 0,
    name: '',
    description: '',
    enabled: true,
    priority: 50,
    target_group: '',
    match_mode: 'all',
    conditions: [{ metric: 'total_spend', op: '>=', value: 0 }],
  };
}

function formatConditionsSummary(conditions, t) {
  const parsed = safeParseJSON(conditions, []);
  if (!parsed.length) return '-';
  return parsed
    .map((c) => {
      const m = METRIC_MAP[c.metric];
      const label = m ? t(m.label) : c.metric;
      const param = m?.needsParam && c.param ? ` (${c.param}h)` : '';
      const displayVal = STRING_METRICS.has(c.metric) ? (c.value_str || '-') : c.value;
      return `${label}${param} ${c.op} ${displayVal}`;
    })
    .join(', ');
}

// ---------------------------------------------------------------------------
// AutoGroup Page
// ---------------------------------------------------------------------------

const AutoGroup = () => {
  const { t } = useTranslation();

  // Tab state
  const [activeTab, setActiveTab] = useState('rules');

  // Rules state
  const [rules, setRules] = useState([]);
  const [rulesTotal, setRulesTotal] = useState(0);
  const [rulesPage, setRulesPage] = useState(1);
  const [rulesKeyword, setRulesKeyword] = useState('');
  const [rulesLoading, setRulesLoading] = useState(false);

  // Enrollments state
  const [enrollments, setEnrollments] = useState([]);
  const [enrollmentsTotal, setEnrollmentsTotal] = useState(0);
  const [enrollmentsPage, setEnrollmentsPage] = useState(1);
  const [enrollmentsKeyword, setEnrollmentsKeyword] = useState('');
  const [enrollmentsLoading, setEnrollmentsLoading] = useState(false);

  // Rule editor SideSheet
  const [ruleSheetVisible, setRuleSheetVisible] = useState(false);
  const [ruleForm, setRuleForm] = useState(emptyRuleForm());
  const [ruleSubmitting, setRuleSubmitting] = useState(false);

  // Enroll modal
  const [enrollVisible, setEnrollVisible] = useState(false);
  const [enrollUserIds, setEnrollUserIds] = useState('');
  const [enrollSubmitting, setEnrollSubmitting] = useState(false);

  // Sweep
  const [sweeping, setSweeping] = useState(false);

  // Group options (for target_group dropdown)
  const [groupOptions, setGroupOptions] = useState([]);
  const [groupDescMap, setGroupDescMap] = useState({});

  // -------------------------------------------------------------------------
  // Data Loading
  // -------------------------------------------------------------------------

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get('/api/user/self/groups');
      if (res.data.success) {
        const data = res.data.data || {};
        const descMap = {};
        const opts = [];
        for (const [key, info] of Object.entries(data)) {
          if (key === 'auto') continue;
          const desc = info?.desc || key;
          descMap[key] = desc;
          opts.push({
            label: desc !== key ? `${key}（${desc}）` : key,
            value: key,
          });
        }
        setGroupOptions(opts);
        setGroupDescMap(descMap);
      }
    } catch {
      // silent
    }
  }, []);

  const loadRules = useCallback(async () => {
    setRulesLoading(true);
    try {
      const params = new URLSearchParams({
        p: String(rulesPage),
        page_size: String(PAGE_SIZE),
      });
      if (rulesKeyword) params.set('keyword', rulesKeyword);
      const res = await API.get(`/api/auto_group/rules?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        setRules(data.items || []);
        setRulesTotal(data.total || 0);
      } else {
        showError(message || t('加载规则失败'));
      }
    } catch (e) {
      showError(t('加载规则失败'));
    } finally {
      setRulesLoading(false);
    }
  }, [rulesPage, rulesKeyword, t]);

  const loadEnrollments = useCallback(async () => {
    setEnrollmentsLoading(true);
    try {
      const params = new URLSearchParams({
        p: String(enrollmentsPage),
        page_size: String(PAGE_SIZE),
      });
      if (enrollmentsKeyword) params.set('keyword', enrollmentsKeyword);
      const res = await API.get(`/api/auto_group/enrollments?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        setEnrollments(data.items || []);
        setEnrollmentsTotal(data.total || 0);
      } else {
        showError(message || t('加载纳入记录失败'));
      }
    } catch {
      showError(t('加载纳入记录失败'));
    } finally {
      setEnrollmentsLoading(false);
    }
  }, [enrollmentsPage, enrollmentsKeyword, t]);

  useEffect(() => {
    loadGroups();
  }, [loadGroups]);

  useEffect(() => {
    loadRules();
  }, [loadRules]);

  useEffect(() => {
    if (activeTab === 'enrollments') {
      loadEnrollments();
    }
  }, [activeTab, loadEnrollments]);

  // -------------------------------------------------------------------------
  // Rule CRUD
  // -------------------------------------------------------------------------

  const openCreateRule = () => {
    setRuleForm(emptyRuleForm());
    setRuleSheetVisible(true);
  };

  const openEditRule = (record) => {
    setRuleForm({
      id: record.id,
      name: record.name || '',
      description: record.description || '',
      enabled: !!record.enabled,
      priority: record.priority ?? 50,
      target_group: record.target_group || '',
      match_mode: record.match_mode || 'all',
      conditions: safeParseJSON(record.conditions, [
        { metric: 'total_spend', op: '>=', value: 0 },
      ]),
    });
    setRuleSheetVisible(true);
  };

  const submitRule = async () => {
    if (!ruleForm.name.trim()) {
      return showError(t('请输入规则名称'));
    }
    if (!ruleForm.target_group) {
      return showError(t('请选择目标分组'));
    }
    if (!ruleForm.conditions.length) {
      return showError(t('至少需要一个条件'));
    }

    setRuleSubmitting(true);
    try {
      const payload = {
        ...ruleForm,
        conditions: JSON.stringify(ruleForm.conditions),
      };

      let res;
      if (ruleForm.id) {
        res = await API.put(`/api/auto_group/rules/${payload.id}`, payload);
      } else {
        res = await API.post('/api/auto_group/rules', payload);
      }

      const { success, message } = res.data;
      if (success) {
        showSuccess(ruleForm.id ? t('规则更新成功') : t('规则创建成功'));
        setRuleSheetVisible(false);
        loadRules();
      } else {
        showError(message || t('保存规则失败'));
      }
    } catch {
      showError(t('保存规则失败'));
    } finally {
      setRuleSubmitting(false);
    }
  };

  const deleteRule = (id) => {
    Modal.confirm({
      title: t('确认删除'),
      content: t('确定要删除此规则吗？'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/auto_group/rules/${id}`);
          if (res.data.success) {
            showSuccess(t('规则删除成功'));
            loadRules();
          } else {
            showError(res.data.message || t('删除规则失败'));
          }
        } catch {
          showError(t('删除规则失败'));
        }
      },
    });
  };

  const toggleRuleEnabled = async (record) => {
    try {
      const payload = {
        ...record,
        enabled: !record.enabled,
        conditions:
          typeof record.conditions === 'string'
            ? record.conditions
            : JSON.stringify(record.conditions),
      };
      const res = await API.put(`/api/auto_group/rules/${payload.id}`, payload);
      if (res.data.success) {
        showSuccess(
          record.enabled ? t('规则已禁用') : t('规则已启用'),
        );
        loadRules();
      } else {
        showError(res.data.message || t('更新规则失败'));
      }
    } catch {
      showError(t('更新规则失败'));
    }
  };

  // -------------------------------------------------------------------------
  // Enrollment actions
  // -------------------------------------------------------------------------

  const submitEnroll = async () => {
    const raw = enrollUserIds.trim();
    if (!raw) return showError(t('请输入用户 ID'));
    const ids = raw
      .split(/[\s,]+/)
      .map((s) => parseInt(s, 10))
      .filter((n) => !isNaN(n) && n > 0);
    if (!ids.length) return showError(t('没有有效的用户 ID'));

    setEnrollSubmitting(true);
    try {
      const res = await API.post('/api/auto_group/enrollments', { user_ids: ids });
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(
          t('已纳入 {{count}} 个用户', { count: data?.count ?? ids.length }),
        );
        setEnrollVisible(false);
        setEnrollUserIds('');
        loadEnrollments();
      } else {
        showError(message || t('纳入用户失败'));
      }
    } catch {
      showError(t('纳入用户失败'));
    } finally {
      setEnrollSubmitting(false);
    }
  };

  const unenroll = (record) => {
    Modal.confirm({
      title: t('确认取消纳入'),
      content: t('确定取消纳入用户 {{name}} 吗？其分组将恢复为原始分组。', {
        name: record.display_name || record.username || record.user_id,
      }),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/auto_group/enrollments/${record.id}`);
          if (res.data.success) {
            showSuccess(t('已取消纳入'));
            loadEnrollments();
          } else {
            showError(res.data.message || t('取消纳入失败'));
          }
        } catch {
          showError(t('取消纳入失败'));
        }
      },
    });
  };

  const triggerSweep = async () => {
    setSweeping(true);
    try {
      const res = await API.post('/api/auto_group/sweep');
      if (res.data.success) {
        showSuccess(t('扫描已触发'));
        loadEnrollments();
      } else {
        showError(res.data.message || t('触发扫描失败'));
      }
    } catch {
      showError(t('触发扫描失败'));
    } finally {
      setSweeping(false);
    }
  };

  // -------------------------------------------------------------------------
  // Form helpers
  // -------------------------------------------------------------------------

  const updateField = (field, value) => {
    setRuleForm((prev) => ({ ...prev, [field]: value }));
  };

  const updateCondition = (index, field, value) => {
    setRuleForm((prev) => ({
      ...prev,
      conditions: prev.conditions.map((c, i) =>
        i === index ? { ...c, [field]: value } : c,
      ),
    }));
  };

  const handleMetricChange = (index, newMetric) => {
    const oldMetric = ruleForm.conditions[index].metric;
    const wasString = STRING_METRICS.has(oldMetric);
    const nowString = STRING_METRICS.has(newMetric);
    setRuleForm((prev) => ({
      ...prev,
      conditions: prev.conditions.map((c, i) => {
        if (i !== index) return c;
        const updated = { ...c, metric: newMetric };
        if (wasString !== nowString) {
          if (nowString) {
            updated.op = '==';
            updated.value = 0;
            updated.value_str = '';
          } else {
            updated.op = '>=';
            updated.value = 0;
            delete updated.value_str;
          }
        }
        return updated;
      }),
    }));
  };

  const addCondition = () => {
    setRuleForm((prev) => ({
      ...prev,
      conditions: [...prev.conditions, { metric: 'total_spend', op: '>=', value: 0 }],
    }));
  };

  const removeCondition = (index) => {
    setRuleForm((prev) => ({
      ...prev,
      conditions: prev.conditions.filter((_, i) => i !== index),
    }));
  };

  // -------------------------------------------------------------------------
  // Rules Table Columns
  // -------------------------------------------------------------------------

  const rulesColumns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: t('名称'), dataIndex: 'name', width: 160 },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      width: 80,
      render: (val) => <Tag>{val}</Tag>,
    },
    {
      title: t('目标分组'),
      dataIndex: 'target_group',
      width: 160,
      render: (val) => {
        const desc = groupDescMap[val];
        return (
          <span>
            <Tag color="blue">{val || '-'}</Tag>
            {desc && desc !== val && (
              <Text type="tertiary" size="small" style={{ marginLeft: 4 }}>
                {desc}
              </Text>
            )}
          </span>
        );
      },
    },
    {
      title: t('匹配模式'),
      dataIndex: 'match_mode',
      width: 100,
      render: (val) => (
        <Tag color={val === 'all' ? 'green' : 'orange'}>
          {val === 'all' ? t('全部') : t('任一')}
        </Tag>
      ),
    },
    {
      title: t('条件'),
      dataIndex: 'conditions',
      render: (val) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 300 }}>
          {formatConditionsSummary(val, t)}
        </Text>
      ),
    },
    {
      title: t('启用'),
      dataIndex: 'enabled',
      width: 80,
      render: (val, record) => (
        <Switch
          checked={!!val}
          onChange={() => toggleRuleEnabled(record)}
          size="small"
        />
      ),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 170,
      render: (val) => (val ? timestamp2string(val) : '-'),
    },
    {
      title: t('操作'),
      width: 140,
      render: (_, record) => (
        <Space>
          <Button size="small" theme="light" onClick={() => openEditRule(record)}>
            {t('编辑')}
          </Button>
          <Button size="small" type="danger" theme="light" onClick={() => deleteRule(record.id)}>
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  // -------------------------------------------------------------------------
  // Enrollments Table Columns
  // -------------------------------------------------------------------------

  const enrollmentsColumns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: t('用户'),
      width: 160,
      render: (_, record) => (
        <span>
          {record.display_name || record.username || '-'}
          <Text type="tertiary" size="small" style={{ marginLeft: 4 }}>
            (ID: {record.user_id})
          </Text>
        </span>
      ),
    },
    {
      title: t('原始分组'),
      dataIndex: 'original_group',
      width: 160,
      render: (val) => {
        const desc = groupDescMap[val];
        return (
          <span>
            <Tag>{val || '-'}</Tag>
            {desc && desc !== val && (
              <Text type="tertiary" size="small" style={{ marginLeft: 4 }}>
                {desc}
              </Text>
            )}
          </span>
        );
      },
    },
    {
      title: t('当前分组'),
      dataIndex: 'current_group',
      width: 160,
      render: (val) => {
        const desc = groupDescMap[val];
        return (
          <span>
            <Tag color="blue">{val || '-'}</Tag>
            {desc && desc !== val && (
              <Text type="tertiary" size="small" style={{ marginLeft: 4 }}>
                {desc}
              </Text>
            )}
          </span>
        );
      },
    },
    {
      title: t('规则 ID'),
      dataIndex: 'current_rule_id',
      width: 80,
      render: (val) => (val ? <Tag>{val}</Tag> : '-'),
    },
    {
      title: t('纳入时间'),
      dataIndex: 'enrolled_at',
      width: 170,
      render: (val) => (val ? timestamp2string(val) : '-'),
    },
    {
      title: t('操作'),
      width: 100,
      render: (_, record) => (
        <Button size="small" type="danger" theme="light" onClick={() => unenroll(record)}>
          {t('取消纳入')}
        </Button>
      ),
    },
  ];

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="mt-[60px] px-2">
      <Tabs activeKey={activeTab} onChange={setActiveTab}>
        {/* ===================== Rules Tab ===================== */}
        <TabPane tab={t('规则')} itemKey="rules">
          <div style={{ marginBottom: 16, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <Input
              placeholder={t('搜索规则...')}
              value={rulesKeyword}
              onChange={(val) => {
                setRulesKeyword(val);
                setRulesPage(1);
              }}
              style={{ width: 240 }}
              showClear
            />
            <Button theme="solid" onClick={openCreateRule}>
              {t('创建规则')}
            </Button>
          </div>
          <Table
            columns={rulesColumns}
            dataSource={rules}
            loading={rulesLoading}
            rowKey="id"
            pagination={{
              currentPage: rulesPage,
              pageSize: PAGE_SIZE,
              total: rulesTotal,
              onPageChange: setRulesPage,
              showSizeChanger: false,
            }}
            empty={<Empty description={t('暂无规则')} />}
          />
        </TabPane>

        {/* ===================== Enrollments Tab ===================== */}
        <TabPane tab={t('纳入记录')} itemKey="enrollments">
          <div style={{ marginBottom: 16, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <Input
              placeholder={t('搜索纳入记录...')}
              value={enrollmentsKeyword}
              onChange={(val) => {
                setEnrollmentsKeyword(val);
                setEnrollmentsPage(1);
              }}
              style={{ width: 240 }}
              showClear
            />
            <Button theme="solid" onClick={() => setEnrollVisible(true)}>
              {t('纳入用户')}
            </Button>
            <Button loading={sweeping} onClick={triggerSweep}>
              {t('触发扫描')}
            </Button>
          </div>
          <Table
            columns={enrollmentsColumns}
            dataSource={enrollments}
            loading={enrollmentsLoading}
            rowKey="id"
            pagination={{
              currentPage: enrollmentsPage,
              pageSize: PAGE_SIZE,
              total: enrollmentsTotal,
              onPageChange: setEnrollmentsPage,
              showSizeChanger: false,
            }}
            empty={<Empty description={t('暂无纳入记录')} />}
          />
        </TabPane>
      </Tabs>

      {/* ===================== Rule Editor SideSheet ===================== */}
      <SideSheet
        title={ruleForm.id ? t('编辑规则') : t('创建规则')}
        visible={ruleSheetVisible}
        onCancel={() => setRuleSheetVisible(false)}
        width={560}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{
          maxHeight: 'calc(100vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => setRuleSheetVisible(false)}>{t('取消')}</Button>
            <Button theme="solid" loading={ruleSubmitting} onClick={submitRule}>
              {ruleForm.id ? t('更新') : t('创建')}
            </Button>
          </div>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {/* Name */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('规则名称')}
            </Text>
            <Input
              value={ruleForm.name}
              onChange={(val) => updateField('name', val)}
              placeholder={t('输入规则名称')}
            />
          </div>

          {/* Description */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('描述')}
            </Text>
            <Input
              value={ruleForm.description}
              onChange={(val) => updateField('description', val)}
              placeholder={t('可选描述')}
            />
          </div>

          {/* Target Group */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('目标分组')}
            </Text>
            <Select
              value={ruleForm.target_group}
              onChange={(val) => updateField('target_group', val)}
              optionList={groupOptions}
              placeholder={t('选择目标分组')}
              style={{ width: '100%' }}
              filter
              getPopupContainer={() => document.body}
            />
          </div>

          {/* Priority + Enabled */}
          <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
            <div style={{ flex: 1 }}>
              <Text strong style={{ marginBottom: 4, display: 'block' }}>
                {t('优先级')}
              </Text>
              <InputNumber
                value={ruleForm.priority}
                onChange={(val) => updateField('priority', val)}
                min={0}
                max={999}
                style={{ width: '100%' }}
              />
            </div>
            <div>
              <Text strong style={{ marginBottom: 4, display: 'block' }}>
                {t('启用')}
              </Text>
              <Switch
                checked={ruleForm.enabled}
                onChange={(val) => updateField('enabled', val)}
              />
            </div>
          </div>

          {/* Match Mode */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('匹配模式')}
            </Text>
            <Select
              value={ruleForm.match_mode}
              onChange={(val) => updateField('match_mode', val)}
              optionList={[
                { label: t('所有条件都需满足'), value: 'all' },
                { label: t('任一条件满足'), value: 'any' },
              ]}
              style={{ width: '100%' }}
              getPopupContainer={() => document.body}
            />
          </div>

          {/* Conditions */}
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <Text strong>{t('条件')}</Text>
              <Button size="small" theme="light" onClick={addCondition}>
                {t('添加条件')}
              </Button>
            </div>
            {ruleForm.conditions.length === 0 && (
              <Empty description={t('暂无条件')} style={{ padding: 16 }} />
            )}
            {ruleForm.conditions.map((condition, index) => {
              const metricConfig = METRIC_MAP[condition.metric];
              const needsParam = metricConfig?.needsParam || false;
              const isString = STRING_METRICS.has(condition.metric);
              return (
                <div
                  key={index}
                  style={{
                    display: 'flex',
                    gap: 8,
                    alignItems: 'center',
                    marginBottom: 8,
                    flexWrap: 'wrap',
                  }}
                >
                  <Select
                    value={condition.metric}
                    onChange={(val) => handleMetricChange(index, val)}
                    optionList={METRIC_OPTIONS.map((o) => ({
                      ...o,
                      label: t(o.label),
                    }))}
                    style={{ width: 200 }}
                    getPopupContainer={() => document.body}
                  />
                  <Select
                    value={condition.op}
                    onChange={(val) => updateCondition(index, 'op', val)}
                    optionList={isString ? STRING_OPS : OP_OPTIONS}
                    style={{ width: 80 }}
                    getPopupContainer={() => document.body}
                  />
                  {isString ? (
                    <Select
                      value={condition.value_str || ''}
                      onChange={(val) => updateCondition(index, 'value_str', val)}
                      optionList={groupOptions.map((o) => ({ label: o.value, value: o.value }))}
                      placeholder={t('选择分组')}
                      style={{ width: 160 }}
                      filter
                      getPopupContainer={() => document.body}
                    />
                  ) : (
                    <InputNumber
                      value={condition.value}
                      onChange={(val) => updateCondition(index, 'value', val)}
                      style={{ width: 100 }}
                    />
                  )}
                  {needsParam && !isString && (
                    <InputNumber
                      value={condition.param}
                      onChange={(val) => updateCondition(index, 'param', val)}
                      min={1}
                      placeholder={t('小时')}
                      style={{ width: 90 }}
                      suffix="h"
                    />
                  )}
                  <Button
                    size="small"
                    type="danger"
                    theme="borderless"
                    onClick={() => removeCondition(index)}
                    disabled={ruleForm.conditions.length <= 1}
                  >
                    {t('移除')}
                  </Button>
                </div>
              );
            })}
          </div>
        </div>
      </SideSheet>

      {/* ===================== Enroll Users Modal ===================== */}
      <Modal
        title={t('纳入用户')}
        visible={enrollVisible}
        onCancel={() => {
          setEnrollVisible(false);
          setEnrollUserIds('');
        }}
        onOk={submitEnroll}
        okText={t('纳入')}
        cancelText={t('取消')}
        confirmLoading={enrollSubmitting}
        centered
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        <div style={{ marginBottom: 8 }}>
          <Text>{t('请输入用户 ID，以逗号或空格分隔：')}</Text>
        </div>
        <Input
          value={enrollUserIds}
          onChange={setEnrollUserIds}
          placeholder={t('例如 1, 2, 3')}
        />
      </Modal>
    </div>
  );
};

export default AutoGroup;

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
  { key: 'total_spend', label: 'Total Spend (USD)', needsParam: false },
  { key: 'recent_spend', label: 'Recent N Hours Spend (USD)', needsParam: true },
  { key: 'yesterday_spend', label: "Yesterday's Spend (USD)", needsParam: false },
  { key: 'total_topup', label: 'Net Top-up (USD)', needsParam: false },
  { key: 'recent_topup', label: 'Recent N Hours Top-up (USD)', needsParam: true },
  { key: 'yesterday_topup', label: "Yesterday's Top-up (USD)", needsParam: false },
  { key: 'total_request_count', label: 'Total Request Count', needsParam: false },
  { key: 'recent_request_count', label: 'Recent N Hours Request Count', needsParam: true },
];

const METRIC_OPTIONS = METRICS.map((m) => ({ label: m.label, value: m.key }));
const METRIC_MAP = Object.fromEntries(METRICS.map((m) => [m.key, m]));

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
      return `${label}${param} ${c.op} ${c.value}`;
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

  // -------------------------------------------------------------------------
  // Data Loading
  // -------------------------------------------------------------------------

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get('/api/group/');
      if (res.data.success) {
        const groups = Object.keys(res.data.data || {});
        setGroupOptions(groups.map((g) => ({ label: g, value: g })));
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
      const res = await API.get(`/api/auto_group_rule/?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        setRules(data.items || []);
        setRulesTotal(data.total || 0);
      } else {
        showError(message || t('Failed to load rules'));
      }
    } catch (e) {
      showError(t('Failed to load rules'));
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
      const res = await API.get(`/api/auto_group_enrollment/?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        setEnrollments(data.items || []);
        setEnrollmentsTotal(data.total || 0);
      } else {
        showError(message || t('Failed to load enrollments'));
      }
    } catch {
      showError(t('Failed to load enrollments'));
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
      return showError(t('Rule name is required'));
    }
    if (!ruleForm.target_group) {
      return showError(t('Target group is required'));
    }
    if (!ruleForm.conditions.length) {
      return showError(t('At least one condition is required'));
    }

    setRuleSubmitting(true);
    try {
      const payload = {
        ...ruleForm,
        conditions: JSON.stringify(ruleForm.conditions),
      };

      let res;
      if (ruleForm.id) {
        res = await API.put('/api/auto_group_rule/', payload);
      } else {
        res = await API.post('/api/auto_group_rule/', payload);
      }

      const { success, message } = res.data;
      if (success) {
        showSuccess(ruleForm.id ? t('Rule updated successfully') : t('Rule created successfully'));
        setRuleSheetVisible(false);
        loadRules();
      } else {
        showError(message || t('Failed to save rule'));
      }
    } catch {
      showError(t('Failed to save rule'));
    } finally {
      setRuleSubmitting(false);
    }
  };

  const deleteRule = (id) => {
    Modal.confirm({
      title: t('Confirm Delete'),
      content: t('Are you sure you want to delete this rule?'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/auto_group_rule/${id}`);
          if (res.data.success) {
            showSuccess(t('Rule deleted successfully'));
            loadRules();
          } else {
            showError(res.data.message || t('Failed to delete rule'));
          }
        } catch {
          showError(t('Failed to delete rule'));
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
      const res = await API.put('/api/auto_group_rule/', payload);
      if (res.data.success) {
        showSuccess(
          record.enabled ? t('Rule disabled successfully') : t('Rule enabled successfully'),
        );
        loadRules();
      } else {
        showError(res.data.message || t('Failed to update rule'));
      }
    } catch {
      showError(t('Failed to update rule'));
    }
  };

  // -------------------------------------------------------------------------
  // Enrollment actions
  // -------------------------------------------------------------------------

  const submitEnroll = async () => {
    const raw = enrollUserIds.trim();
    if (!raw) return showError(t('Please enter user IDs'));
    const ids = raw
      .split(/[\s,]+/)
      .map((s) => parseInt(s, 10))
      .filter((n) => !isNaN(n) && n > 0);
    if (!ids.length) return showError(t('No valid user IDs'));

    setEnrollSubmitting(true);
    try {
      const res = await API.post('/api/auto_group_enrollment/', { user_ids: ids });
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(
          t('Enrolled {{count}} user(s)', { count: data?.count ?? ids.length }),
        );
        setEnrollVisible(false);
        setEnrollUserIds('');
        loadEnrollments();
      } else {
        showError(message || t('Failed to enroll users'));
      }
    } catch {
      showError(t('Failed to enroll users'));
    } finally {
      setEnrollSubmitting(false);
    }
  };

  const unenroll = (record) => {
    Modal.confirm({
      title: t('Confirm Unenroll'),
      content: t('Unenroll user {{name}}? Their group will revert to the original group.', {
        name: record.display_name || record.username || record.user_id,
      }),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.delete(`/api/auto_group_enrollment/${record.id}`);
          if (res.data.success) {
            showSuccess(t('User unenrolled successfully'));
            loadEnrollments();
          } else {
            showError(res.data.message || t('Failed to unenroll user'));
          }
        } catch {
          showError(t('Failed to unenroll user'));
        }
      },
    });
  };

  const triggerSweep = async () => {
    setSweeping(true);
    try {
      const res = await API.post('/api/auto_group_enrollment/sweep');
      if (res.data.success) {
        showSuccess(t('Sweep triggered successfully'));
        loadEnrollments();
      } else {
        showError(res.data.message || t('Failed to trigger sweep'));
      }
    } catch {
      showError(t('Failed to trigger sweep'));
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
    { title: t('Name'), dataIndex: 'name', width: 160 },
    {
      title: t('Priority'),
      dataIndex: 'priority',
      width: 80,
      render: (val) => <Tag>{val}</Tag>,
    },
    {
      title: t('Target Group'),
      dataIndex: 'target_group',
      width: 120,
      render: (val) => <Tag color="blue">{val || '-'}</Tag>,
    },
    {
      title: t('Match Mode'),
      dataIndex: 'match_mode',
      width: 100,
      render: (val) => (
        <Tag color={val === 'all' ? 'green' : 'orange'}>
          {val === 'all' ? t('All') : t('Any')}
        </Tag>
      ),
    },
    {
      title: t('Conditions'),
      dataIndex: 'conditions',
      render: (val) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 300 }}>
          {formatConditionsSummary(val, t)}
        </Text>
      ),
    },
    {
      title: t('Enabled'),
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
      title: t('Created At'),
      dataIndex: 'created_at',
      width: 170,
      render: (val) => (val ? timestamp2string(val) : '-'),
    },
    {
      title: t('Actions'),
      width: 140,
      render: (_, record) => (
        <Space>
          <Button size="small" theme="light" onClick={() => openEditRule(record)}>
            {t('Edit')}
          </Button>
          <Button size="small" type="danger" theme="light" onClick={() => deleteRule(record.id)}>
            {t('Delete')}
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
      title: t('User'),
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
      title: t('Original Group'),
      dataIndex: 'original_group',
      width: 120,
      render: (val) => <Tag>{val || '-'}</Tag>,
    },
    {
      title: t('Current Group'),
      dataIndex: 'current_group',
      width: 120,
      render: (val) => <Tag color="blue">{val || '-'}</Tag>,
    },
    {
      title: t('Rule ID'),
      dataIndex: 'current_rule_id',
      width: 80,
      render: (val) => (val ? <Tag>{val}</Tag> : '-'),
    },
    {
      title: t('Enrolled At'),
      dataIndex: 'enrolled_at',
      width: 170,
      render: (val) => (val ? timestamp2string(val) : '-'),
    },
    {
      title: t('Actions'),
      width: 100,
      render: (_, record) => (
        <Button size="small" type="danger" theme="light" onClick={() => unenroll(record)}>
          {t('Unenroll')}
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
        <TabPane tab={t('Rules')} itemKey="rules">
          <div style={{ marginBottom: 16, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <Input
              placeholder={t('Search rules...')}
              value={rulesKeyword}
              onChange={(val) => {
                setRulesKeyword(val);
                setRulesPage(1);
              }}
              style={{ width: 240 }}
              showClear
            />
            <Button theme="solid" onClick={openCreateRule}>
              {t('Create Rule')}
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
            empty={<Empty description={t('No rules')} />}
          />
        </TabPane>

        {/* ===================== Enrollments Tab ===================== */}
        <TabPane tab={t('Enrollments')} itemKey="enrollments">
          <div style={{ marginBottom: 16, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <Input
              placeholder={t('Search enrollments...')}
              value={enrollmentsKeyword}
              onChange={(val) => {
                setEnrollmentsKeyword(val);
                setEnrollmentsPage(1);
              }}
              style={{ width: 240 }}
              showClear
            />
            <Button theme="solid" onClick={() => setEnrollVisible(true)}>
              {t('Enroll Users')}
            </Button>
            <Button loading={sweeping} onClick={triggerSweep}>
              {t('Trigger Sweep')}
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
            empty={<Empty description={t('No enrollments')} />}
          />
        </TabPane>
      </Tabs>

      {/* ===================== Rule Editor SideSheet ===================== */}
      <SideSheet
        title={ruleForm.id ? t('Edit Rule') : t('Create Rule')}
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
            <Button onClick={() => setRuleSheetVisible(false)}>{t('Cancel')}</Button>
            <Button theme="solid" loading={ruleSubmitting} onClick={submitRule}>
              {ruleForm.id ? t('Update') : t('Create')}
            </Button>
          </div>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {/* Name */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('Rule Name')}
            </Text>
            <Input
              value={ruleForm.name}
              onChange={(val) => updateField('name', val)}
              placeholder={t('Enter rule name')}
            />
          </div>

          {/* Description */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('Description')}
            </Text>
            <Input
              value={ruleForm.description}
              onChange={(val) => updateField('description', val)}
              placeholder={t('Optional description')}
            />
          </div>

          {/* Target Group */}
          <div>
            <Text strong style={{ marginBottom: 4, display: 'block' }}>
              {t('Target Group')}
            </Text>
            <Select
              value={ruleForm.target_group}
              onChange={(val) => updateField('target_group', val)}
              optionList={groupOptions}
              placeholder={t('Select target group')}
              style={{ width: '100%' }}
              filter
              getPopupContainer={() => document.body}
            />
          </div>

          {/* Priority + Enabled */}
          <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
            <div style={{ flex: 1 }}>
              <Text strong style={{ marginBottom: 4, display: 'block' }}>
                {t('Priority')}
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
                {t('Enabled')}
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
              {t('Match Mode')}
            </Text>
            <Select
              value={ruleForm.match_mode}
              onChange={(val) => updateField('match_mode', val)}
              optionList={[
                { label: t('All conditions must match'), value: 'all' },
                { label: t('Any condition matches'), value: 'any' },
              ]}
              style={{ width: '100%' }}
              getPopupContainer={() => document.body}
            />
          </div>

          {/* Conditions */}
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <Text strong>{t('Conditions')}</Text>
              <Button size="small" theme="light" onClick={addCondition}>
                {t('Add Condition')}
              </Button>
            </div>
            {ruleForm.conditions.length === 0 && (
              <Empty description={t('No conditions added')} style={{ padding: 16 }} />
            )}
            {ruleForm.conditions.map((condition, index) => {
              const metricConfig = METRIC_MAP[condition.metric];
              const needsParam = metricConfig?.needsParam || false;
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
                    onChange={(val) => updateCondition(index, 'metric', val)}
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
                    optionList={OP_OPTIONS}
                    style={{ width: 80 }}
                    getPopupContainer={() => document.body}
                  />
                  <InputNumber
                    value={condition.value}
                    onChange={(val) => updateCondition(index, 'value', val)}
                    style={{ width: 100 }}
                  />
                  {needsParam && (
                    <InputNumber
                      value={condition.param}
                      onChange={(val) => updateCondition(index, 'param', val)}
                      min={1}
                      placeholder={t('Hours')}
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
                    {t('Remove')}
                  </Button>
                </div>
              );
            })}
          </div>
        </div>
      </SideSheet>

      {/* ===================== Enroll Users Modal ===================== */}
      <Modal
        title={t('Enroll Users')}
        visible={enrollVisible}
        onCancel={() => {
          setEnrollVisible(false);
          setEnrollUserIds('');
        }}
        onOk={submitEnroll}
        okText={t('Enroll')}
        cancelText={t('Cancel')}
        confirmLoading={enrollSubmitting}
        centered
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        <div style={{ marginBottom: 8 }}>
          <Text>{t('Enter user IDs separated by commas or spaces:')}</Text>
        </div>
        <Input
          value={enrollUserIds}
          onChange={setEnrollUserIds}
          placeholder={t('e.g. 1, 2, 3')}
        />
      </Modal>
    </div>
  );
};

export default AutoGroup;

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
  Form,
  Modal,
  Row,
  Select,
  Spin,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { Plus, Users, Mail, Paperclip, Share2 } from 'lucide-react';

import { API, showError, showSuccess, toBoolean } from '../../helpers';

const { Text } = Typography;

// 三种基础工单类型（与后端 model.TicketType* 对齐）。
// 如果未来新增工单类型，只需在这里追加一项并同步后端常量。
const TICKET_TYPES = [
  { value: 'general', labelKey: '普通工单', descKey: '默认类型，涵盖大多数咨询与报错反馈。' },
  { value: 'refund', labelKey: '退款处理', descKey: '涉及资金返还，建议分配给熟悉财务流程的客服。' },
  { value: 'invoice', labelKey: '发票处理', descKey: '涉及开票信息核对，建议分配给熟悉发票业务的客服。' },
];

const STRATEGY_OPTIONS = (t) => [
  {
    value: 'round_robin',
    label: t('轮询'),
    hint: t('按固定顺序依次分配，工单量均衡时首选。'),
  },
  {
    value: 'least_loaded',
    label: t('最少负载'),
    hint: t('优先分配给当前未关闭工单最少的客服，避免个别客服堆积。'),
  },
  {
    value: 'random',
    label: t('随机'),
    hint: t('随机选择一位，实现最简单但分布不一定均衡。'),
  },
  {
    value: 'manual',
    label: t('仅手动分配'),
    hint: t('新工单不会被自动分配，管理员需要在列表中手动指派。'),
  },
];

const FALLBACK_OPTIONS = (t) => [
  {
    value: 'none',
    label: t('不回落（留待认领池）'),
    hint: t('该类型没有可用客服时，工单保留在"待认领"池中，仅通知管理员。'),
  },
  {
    value: 'general_group',
    label: t('回落到"普通工单"组'),
    hint: t('当退款/发票组无可用客服时，自动交给普通工单组兜底处理。'),
  },
];

const roleLabel = (role, t) => {
  if (role >= 100) return t('超级管理员');
  if (role >= 10) return t('管理员');
  if (role >= 5) return t('客服');
  return t('普通用户');
};

const TicketSetting = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [savingAssign, setSavingAssign] = useState(false);

  // Assignment 配置：与后端 setting.TicketAssignConfig 结构一致。
  const [assignConfig, setAssignConfig] = useState({
    enabled: false,
    fallback: 'none',
    rules: {
      general: { strategy: 'round_robin', users: [] },
      refund: { strategy: 'round_robin', users: [] },
      invoice: { strategy: 'round_robin', users: [] },
    },
  });
  const [staffList, setStaffList] = useState([]);

  // 通知 + 附件相关字段，整体保持与原 SystemSetting 对齐。
  const [inputs, setInputs] = useState({
    TicketNotifyEnabled: false,
    TicketAdminEmail: '',
    TicketAttachmentEnabled: true,
    TicketAttachmentMaxSize: '52428800',
    TicketAttachmentMaxCount: '5',
    TicketAttachmentAllowedExts: '',
    TicketAttachmentAllowedMimes: '',
    TicketAttachmentStorage: 'local',
    TicketAttachmentLocalPath: '',
    TicketAttachmentSignedURLTTL: '900',
    TicketAttachmentOSSEndpoint: '',
    TicketAttachmentOSSBucket: '',
    TicketAttachmentOSSRegion: '',
    TicketAttachmentOSSAccessKeyId: '',
    TicketAttachmentOSSAccessKeySecret: '',
    TicketAttachmentOSSCustomDomain: '',
    TicketAttachmentS3Endpoint: '',
    TicketAttachmentS3Bucket: '',
    TicketAttachmentS3Region: '',
    TicketAttachmentS3AccessKeyId: '',
    TicketAttachmentS3AccessKeySecret: '',
    TicketAttachmentS3CustomDomain: '',
    TicketAttachmentCOSEndpoint: '',
    TicketAttachmentCOSBucket: '',
    TicketAttachmentCOSRegion: '',
    TicketAttachmentCOSSecretId: '',
    TicketAttachmentCOSSecretKey: '',
    TicketAttachmentCOSCustomDomain: '',
  });

  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerTargetType, setPickerTargetType] = useState(null);
  const [pickerSelected, setPickerSelected] = useState([]);

  const loadOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (!success) {
      showError(t(message));
      return;
    }
    const next = {};
    let nextAssignRaw = null;
    data.forEach((item) => {
      const k = item.key;
      const v = item.value;
      if (k === 'TicketAssignConfig') {
        nextAssignRaw = v;
        return;
      }
      if (!k.startsWith('Ticket')) return;
      if (k.endsWith('Enabled')) {
        next[k] = toBoolean(v);
      } else {
        next[k] = v;
      }
    });
    setInputs((prev) => ({ ...prev, ...next }));
    if (nextAssignRaw) {
      try {
        const parsed = JSON.parse(nextAssignRaw);
        if (parsed && typeof parsed === 'object') {
          setAssignConfig({
            enabled: !!parsed.enabled,
            fallback: parsed.fallback || 'none',
            rules: {
              general: { strategy: 'round_robin', users: [], ...(parsed.rules?.general || {}) },
              refund: { strategy: 'round_robin', users: [], ...(parsed.rules?.refund || {}) },
              invoice: { strategy: 'round_robin', users: [], ...(parsed.rules?.invoice || {}) },
            },
          });
        }
      } catch {
        // 配置损坏时保持默认值，避免页面崩溃。
      }
    }
  };

  const loadStaff = async () => {
    try {
      const res = await API.get('/api/ticket/admin/staff');
      if (res.data?.success) {
        setStaffList(res.data.data || []);
      }
    } catch (err) {
      // 未启用管理员/客服权限的账号访问该接口会被 403，这里静默忽略
    }
  };

  const onRefresh = async () => {
    try {
      setLoading(true);
      await Promise.all([loadOptions(), loadStaff()]);
    } catch (err) {
      showError(t('刷新失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    onRefresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const staffIndex = useMemo(() => {
    const m = new Map();
    staffList.forEach((u) => m.set(u.id, u));
    return m;
  }, [staffList]);

  const assignedUserIds = useMemo(() => {
    const s = new Set();
    Object.values(assignConfig.rules || {}).forEach((rule) => {
      (rule.users || []).forEach((uid) => s.add(uid));
    });
    return s;
  }, [assignConfig]);

  const unassignedStaff = useMemo(() => {
    return staffList.filter((u) => !assignedUserIds.has(u.id));
  }, [staffList, assignedUserIds]);

  const saveAssignConfig = async (next) => {
    const body = next || assignConfig;
    setSavingAssign(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'TicketAssignConfig',
        value: JSON.stringify(body),
      });
      if (res.data?.success) {
        showSuccess(t('分配规则已保存'));
        setAssignConfig(body);
      } else {
        showError(t(res.data?.message || '保存失败'));
      }
    } catch (err) {
      showError(t('保存失败'));
    } finally {
      setSavingAssign(false);
    }
  };

  const updateRule = (type, patch) => {
    const nextRule = { ...assignConfig.rules[type], ...patch };
    const nextCfg = {
      ...assignConfig,
      rules: { ...assignConfig.rules, [type]: nextRule },
    };
    setAssignConfig(nextCfg);
  };

  const removeUserFromRule = (type, userId) => {
    const nextUsers = (assignConfig.rules[type]?.users || []).filter((u) => u !== userId);
    updateRule(type, { users: nextUsers });
  };

  const openPicker = (type) => {
    setPickerTargetType(type);
    setPickerSelected([...(assignConfig.rules[type]?.users || [])]);
    setPickerOpen(true);
  };

  const confirmPicker = () => {
    if (!pickerTargetType) return;
    updateRule(pickerTargetType, { users: [...new Set(pickerSelected)].sort((a, b) => a - b) });
    setPickerOpen(false);
  };

  const updateOptions = async (options) => {
    const promises = options.map(async (opt) => {
      try {
        const res = await API.put('/api/option/', opt);
        return res.data?.success !== false;
      } catch {
        return false;
      }
    });
    const results = await Promise.all(promises);
    if (results.every(Boolean)) {
      showSuccess(t('保存成功'));
    } else {
      showError(t('部分配置保存失败'));
    }
  };

  const submitNotify = async () => {
    await updateOptions([
      { key: 'TicketNotifyEnabled', value: inputs.TicketNotifyEnabled ? 'true' : 'false' },
      { key: 'TicketAdminEmail', value: (inputs.TicketAdminEmail || '').trim() },
    ]);
  };

  const submitAttachment = async () => {
    const pick = (key) => ({ key, value: String(inputs[key] ?? '').trim() });
    const storage = String(inputs.TicketAttachmentStorage || 'local');
    const options = [
      { key: 'TicketAttachmentEnabled', value: inputs.TicketAttachmentEnabled ? 'true' : 'false' },
      pick('TicketAttachmentMaxSize'),
      pick('TicketAttachmentMaxCount'),
      pick('TicketAttachmentAllowedExts'),
      pick('TicketAttachmentAllowedMimes'),
      { key: 'TicketAttachmentStorage', value: storage },
      pick('TicketAttachmentLocalPath'),
      pick('TicketAttachmentSignedURLTTL'),
    ];
    if (storage === 'oss') {
      options.push(
        pick('TicketAttachmentOSSEndpoint'),
        pick('TicketAttachmentOSSBucket'),
        pick('TicketAttachmentOSSRegion'),
        pick('TicketAttachmentOSSAccessKeyId'),
        pick('TicketAttachmentOSSAccessKeySecret'),
        pick('TicketAttachmentOSSCustomDomain'),
      );
    } else if (storage === 's3') {
      options.push(
        pick('TicketAttachmentS3Endpoint'),
        pick('TicketAttachmentS3Bucket'),
        pick('TicketAttachmentS3Region'),
        pick('TicketAttachmentS3AccessKeyId'),
        pick('TicketAttachmentS3AccessKeySecret'),
        pick('TicketAttachmentS3CustomDomain'),
      );
    } else if (storage === 'cos') {
      options.push(
        pick('TicketAttachmentCOSEndpoint'),
        pick('TicketAttachmentCOSBucket'),
        pick('TicketAttachmentCOSRegion'),
        pick('TicketAttachmentCOSSecretId'),
        pick('TicketAttachmentCOSSecretKey'),
        pick('TicketAttachmentCOSCustomDomain'),
      );
    }
    await updateOptions(options);
  };

  const handleInputChange = (key, value) => {
    setInputs((prev) => ({ ...prev, [key]: value }));
  };

  return (
    <Spin spinning={loading} size='large'>
      <Card style={{ marginTop: '10px' }}>
        <Tabs type='card' defaultActiveKey='assignment' contentStyle={{ paddingTop: 24 }}>
          {/* ============ 分配规则 Tab ============ */}
          <Tabs.TabPane
            itemKey='assignment'
            tab={
              <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <Share2 size={16} />
                {t('分配规则')}
              </span>
            }
          >
            <AssignmentPane
              t={t}
              config={assignConfig}
              setConfig={setAssignConfig}
              staffList={staffList}
              staffIndex={staffIndex}
              unassignedStaff={unassignedStaff}
              saving={savingAssign}
              onSave={() => saveAssignConfig()}
              updateRule={updateRule}
              removeUserFromRule={removeUserFromRule}
              openPicker={openPicker}
              roleLabel={roleLabel}
            />
          </Tabs.TabPane>

          {/* ============ 邮件通知 Tab ============ */}
          <Tabs.TabPane
            itemKey='notify'
            tab={
              <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <Mail size={16} />
                {t('邮件通知')}
              </span>
            }
          >
            <NotifyPane
              t={t}
              inputs={inputs}
              onChange={handleInputChange}
              onSubmit={submitNotify}
            />
          </Tabs.TabPane>

          {/* ============ 附件 Tab ============ */}
          <Tabs.TabPane
            itemKey='attachment'
            tab={
              <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <Paperclip size={16} />
                {t('附件设置')}
              </span>
            }
          >
            <AttachmentPane
              t={t}
              inputs={inputs}
              onChange={handleInputChange}
              onSubmit={submitAttachment}
            />
          </Tabs.TabPane>
        </Tabs>
      </Card>

      {/* 分配成员的选择弹窗 */}
      <StaffPickerModal
        t={t}
        visible={pickerOpen}
        onCancel={() => setPickerOpen(false)}
        onConfirm={confirmPicker}
        staffList={staffList}
        selected={pickerSelected}
        setSelected={setPickerSelected}
        roleLabel={roleLabel}
      />
    </Spin>
  );
};

// =====================================================
// 分配规则面板
// =====================================================
const AssignmentPane = ({
  t,
  config,
  setConfig,
  staffList,
  staffIndex,
  unassignedStaff,
  saving,
  onSave,
  updateRule,
  removeUserFromRule,
  openPicker,
  roleLabel,
}) => {
  const strategyOptions = STRATEGY_OPTIONS(t);
  const fallbackOptions = FALLBACK_OPTIONS(t);

  return (
    <div>
      <Banner
        fullMode={false}
        type='info'
        closeIcon={null}
        title={t('如何配置工单分配')}
        description={
          <ul style={{ margin: '4px 0 0 16px', padding: 0 }}>
            <li>{t('把希望接收工单的账号角色设为"客服"或更高，他们就会出现在下方"可分配客服"列表。')}</li>
            <li>{t('拖动或点击按钮把客服加入到不同的工单类型分组，一位客服可以同时出现在多个分组中（一人多职）。')}</li>
            <li>{t('每个分组可以独立选择分配策略（轮询 / 最少负载 / 随机 / 手动）。')}</li>
            <li>{t('工单邮件会直接发到客服本人绑定的邮箱，无需在下方"邮件通知"里为他们单独配置。管理员邮箱会同时抄送分配结果。')}</li>
            <li>{t('当某类型的分组里没有可用客服时，会按"兜底策略"处理；或留在"待认领"池由管理员手动指派。')}</li>
          </ul>
        }
        style={{ marginBottom: 16 }}
      />

      <Card style={{ marginBottom: 16 }}>
        <Row gutter={16} type='flex' align='middle'>
          <Col xs={24} md={8}>
            <Space>
              <Switch
                checked={!!config.enabled}
                onChange={(v) => setConfig({ ...config, enabled: v })}
              />
              <Text strong>{t('启用自动分配')}</Text>
            </Space>
            <div style={{ marginTop: 4, fontSize: 12, color: 'var(--semi-color-text-2)' }}>
              {t('关闭后所有新工单都会留在待认领池。')}
            </div>
          </Col>
          <Col xs={24} md={10}>
            <div style={{ fontSize: 12, color: 'var(--semi-color-text-2)', marginBottom: 4 }}>
              {t('兜底策略')}
            </div>
            <Select
              style={{ width: '100%' }}
              value={config.fallback}
              optionList={fallbackOptions.map((o) => ({ value: o.value, label: o.label }))}
              onChange={(v) => setConfig({ ...config, fallback: v })}
              getPopupContainer={() => document.body}
            />
            <div style={{ marginTop: 4, fontSize: 12, color: 'var(--semi-color-text-2)' }}>
              {fallbackOptions.find((o) => o.value === config.fallback)?.hint}
            </div>
          </Col>
          <Col xs={24} md={6} style={{ textAlign: 'right' }}>
            <Button theme='solid' type='primary' loading={saving} onClick={onSave}>
              {t('保存分配设置')}
            </Button>
          </Col>
        </Row>
      </Card>

      {/* 未加入任何分组的候选客服 */}
      <Card
        style={{ marginBottom: 16 }}
        title={
          <Space>
            <Users size={16} />
            <Text strong>{t('未加入任何分组的客服')}</Text>
          </Space>
        }
      >
        {staffList.length === 0 ? (
          <Text type='tertiary'>
            {t('还没有角色为"客服"或更高的账号。请先在"用户"页面把员工升级为客服。')}
          </Text>
        ) : unassignedStaff.length === 0 ? (
          <Text type='tertiary'>{t('所有客服都已加入至少一个分组。')}</Text>
        ) : (
          <Space wrap>
            {unassignedStaff.map((u) => (
              <Tag key={u.id} color='grey' size='large'>
                {u.display_name || u.username}
                <span style={{ marginLeft: 4, color: 'var(--semi-color-text-2)', fontSize: 11 }}>
                  · {roleLabel(u.role, t)}
                </span>
              </Tag>
            ))}
          </Space>
        )}
        <Divider margin='12px' />
        <Text type='tertiary' style={{ fontSize: 12 }}>
          {t('这里只展示尚未出现在任何工单分组里的客服；一旦把他们加入某个分组，就会从这里消失。')}
        </Text>
      </Card>

      {/* 三组工单类型的成员配置 */}
      {TICKET_TYPES.map((type) => {
        const rule = config.rules[type.value] || { strategy: 'round_robin', users: [] };
        const strategyDesc = strategyOptions.find((o) => o.value === rule.strategy)?.hint;
        return (
          <Card
            key={type.value}
            style={{ marginBottom: 16 }}
            title={
              <Row style={{ width: '100%' }} align='middle' justify='space-between'>
                <Col>
                  <Text strong style={{ fontSize: 15 }}>
                    {t(type.labelKey)}
                  </Text>
                  <Text type='tertiary' style={{ marginLeft: 8, fontSize: 12 }}>
                    {t(type.descKey)}
                  </Text>
                </Col>
              </Row>
            }
          >
            <Row gutter={16} style={{ marginBottom: 12 }}>
              <Col xs={24} md={10}>
                <div style={{ fontSize: 12, color: 'var(--semi-color-text-2)', marginBottom: 4 }}>
                  {t('分配策略')}
                </div>
                <Select
                  style={{ width: '100%' }}
                  value={rule.strategy}
                  optionList={strategyOptions.map((o) => ({ value: o.value, label: o.label }))}
                  onChange={(v) => updateRule(type.value, { strategy: v })}
                  getPopupContainer={() => document.body}
                />
                <div style={{ marginTop: 4, fontSize: 12, color: 'var(--semi-color-text-2)' }}>
                  {strategyDesc}
                </div>
              </Col>
              <Col xs={24} md={14} style={{ display: 'flex', alignItems: 'flex-end', justifyContent: 'flex-end' }}>
                <Button
                  icon={<Plus size={14} />}
                  theme='light'
                  onClick={() => openPicker(type.value)}
                >
                  {t('添加客服')}
                </Button>
              </Col>
            </Row>

            <div>
              <Text strong style={{ marginRight: 8 }}>
                {t('成员')}
                <span style={{ color: 'var(--semi-color-text-2)', fontWeight: 400 }}>
                  （{(rule.users || []).length}）
                </span>
              </Text>
            </div>
            <div style={{ marginTop: 8, minHeight: 32 }}>
              {(rule.users || []).length === 0 ? (
                <Text type='tertiary' style={{ fontSize: 12 }}>
                  {t('当前无成员。点击右上角"添加客服"挑选。')}
                </Text>
              ) : (
                <Space wrap>
                  {(rule.users || []).map((uid) => {
                    const u = staffIndex.get(uid);
                    const label = u ? u.display_name || u.username : `#${uid}`;
                    const missing = !u;
                    return (
                      <Tag
                        key={uid}
                        size='large'
                        color={missing ? 'red' : 'blue'}
                        closable
                        onClose={(e) => {
                          e?.preventDefault?.();
                          removeUserFromRule(type.value, uid);
                        }}
                      >
                        {label}
                        {u && (
                          <span style={{ marginLeft: 4, color: 'var(--semi-color-text-2)', fontSize: 11 }}>
                            · {roleLabel(u.role, t)}
                          </span>
                        )}
                        {missing && (
                          <span style={{ marginLeft: 4, fontSize: 11 }}>
                            ({t('账号不存在或已被禁用')})
                          </span>
                        )}
                      </Tag>
                    );
                  })}
                </Space>
              )}
            </div>
          </Card>
        );
      })}
    </div>
  );
};

// =====================================================
// 邮件通知面板
// =====================================================
const NotifyPane = ({ t, inputs, onChange, onSubmit }) => {
  return (
    <div>
      <Banner
        fullMode={false}
        type='info'
        closeIcon={null}
        title={t('工单邮件通知')}
        description={t(
          '依赖上方 SMTP 配置。启用后，用户创建工单时会通知管理员邮箱；管理员回复工单时会通知用户绑定邮箱。客服无需单独配置收件地址——系统会使用客服账号本身的邮箱收件。',
        )}
        style={{ marginBottom: 16 }}
      />

      <Form layout='vertical'>
        <Row gutter={24}>
          <Col xs={24} md={8}>
            <Form.Slot label={t('启用工单邮件通知')}>
              <Switch
                checked={!!inputs.TicketNotifyEnabled}
                onChange={(v) => onChange('TicketNotifyEnabled', v)}
              />
            </Form.Slot>
          </Col>
          <Col xs={24} md={16}>
            <Form.Slot
              label={t('管理员收件邮箱')}
              extraText={t(
                '多个邮箱使用分号(;)或逗号分隔。用于"新工单""分配结果""用户追加回复"等管理员侧邮件。',
              )}
            >
              <Form.Input
                noLabel
                value={inputs.TicketAdminEmail}
                placeholder={t('例如 ops@example.com; support@example.com')}
                onChange={(v) => onChange('TicketAdminEmail', v)}
              />
            </Form.Slot>
          </Col>
        </Row>
        <Button theme='solid' type='primary' onClick={onSubmit}>
          {t('保存工单通知设置')}
        </Button>
      </Form>
    </div>
  );
};

// =====================================================
// 附件设置面板
// =====================================================
const AttachmentPane = ({ t, inputs, onChange, onSubmit }) => {
  return (
    <div>
      <Banner
        fullMode={false}
        type='info'
        closeIcon={null}
        title={t('工单附件')}
        description={t(
          '允许用户和管理员在工单中上传图片、JSON/XML/TXT/PDF 等附件。出于安全考虑 SVG 被强制禁用；默认本地磁盘存储，也可切换到阿里云 OSS / AWS S3 / 腾讯 COS。',
        )}
        style={{ marginBottom: 16 }}
      />
      <Form layout='vertical'>
        <Row gutter={24}>
          <Col xs={24} sm={12} md={6}>
            <Form.Slot label={t('启用工单附件')}>
              <Switch
                checked={!!inputs.TicketAttachmentEnabled}
                onChange={(v) => onChange('TicketAttachmentEnabled', v)}
              />
            </Form.Slot>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Form.Slot label={t('单文件上限（字节）')}>
              <Form.Input
                noLabel
                value={inputs.TicketAttachmentMaxSize}
                placeholder='52428800'
                onChange={(v) => onChange('TicketAttachmentMaxSize', v)}
              />
            </Form.Slot>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Form.Slot label={t('单条消息附件数上限')}>
              <Form.Input
                noLabel
                value={inputs.TicketAttachmentMaxCount}
                placeholder='5'
                onChange={(v) => onChange('TicketAttachmentMaxCount', v)}
              />
            </Form.Slot>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Form.Slot label={t('签名 URL 有效期（秒）')}>
              <Form.Input
                noLabel
                value={inputs.TicketAttachmentSignedURLTTL}
                placeholder='900'
                onChange={(v) => onChange('TicketAttachmentSignedURLTTL', v)}
              />
            </Form.Slot>
          </Col>
        </Row>
        <Row gutter={24}>
          <Col xs={24} md={12}>
            <Form.Slot label={t('允许的扩展名（逗号分隔，小写）')}>
              <Form.Input
                noLabel
                value={inputs.TicketAttachmentAllowedExts}
                placeholder='jpg,jpeg,png,gif,webp,json,xml,txt,log,md,csv,pdf'
                onChange={(v) => onChange('TicketAttachmentAllowedExts', v)}
              />
            </Form.Slot>
          </Col>
          <Col xs={24} md={12}>
            <Form.Slot label={t('允许的 MIME（支持 type/* 通配符）')}>
              <Form.Input
                noLabel
                value={inputs.TicketAttachmentAllowedMimes}
                placeholder='image/*,application/json,application/xml,text/*,application/pdf'
                onChange={(v) => onChange('TicketAttachmentAllowedMimes', v)}
              />
            </Form.Slot>
          </Col>
        </Row>
        <Row gutter={24}>
          <Col xs={24} sm={12} md={8}>
            <Form.Slot label={t('存储后端')}>
              <Select
                style={{ width: '100%' }}
                value={inputs.TicketAttachmentStorage}
                onChange={(v) => onChange('TicketAttachmentStorage', v)}
                optionList={[
                  { label: t('本地磁盘'), value: 'local' },
                  { label: t('阿里云 OSS'), value: 'oss' },
                  { label: t('AWS S3'), value: 's3' },
                  { label: t('腾讯云 COS'), value: 'cos' },
                ]}
                getPopupContainer={() => document.body}
              />
            </Form.Slot>
          </Col>
          {inputs.TicketAttachmentStorage === 'local' && (
            <Col xs={24} sm={12} md={16}>
              <Form.Slot label={t('本地存储根目录')}>
                <Form.Input
                  noLabel
                  value={inputs.TicketAttachmentLocalPath}
                  placeholder='data/ticket_attachments'
                  onChange={(v) => onChange('TicketAttachmentLocalPath', v)}
                />
              </Form.Slot>
            </Col>
          )}
        </Row>

        {inputs.TicketAttachmentStorage === 'oss' && (
          <CloudStorageFields
            t={t}
            prefix='OSS'
            inputs={inputs}
            onChange={onChange}
            endpointHint='oss-cn-hangzhou.aliyuncs.com'
            regionHint='cn-hangzhou'
            idLabel='AccessKey ID'
            secretLabel='AccessKey Secret'
          />
        )}
        {inputs.TicketAttachmentStorage === 's3' && (
          <CloudStorageFields
            t={t}
            prefix='S3'
            inputs={inputs}
            onChange={onChange}
            endpointLabel={t('S3 Endpoint（可选，MinIO/R2 使用）')}
            endpointHint='https://s3.amazonaws.com'
            regionHint='us-east-1'
            idLabel='AccessKey ID'
            secretLabel='AccessKey Secret'
          />
        )}
        {inputs.TicketAttachmentStorage === 'cos' && (
          <CloudStorageFields
            t={t}
            prefix='COS'
            inputs={inputs}
            onChange={onChange}
            endpointLabel={t('COS 完整 Bucket URL（可选）')}
            endpointHint='https://<bucket>.cos.<region>.myqcloud.com'
            regionHint='ap-guangzhou'
            idLabel='SecretId'
            secretLabel='SecretKey'
            idField='SecretId'
            secretField='SecretKey'
          />
        )}

        <Button theme='solid' type='primary' onClick={onSubmit}>
          {t('保存工单附件设置')}
        </Button>
      </Form>
    </div>
  );
};

const CloudStorageFields = ({
  t,
  prefix,
  inputs,
  onChange,
  endpointHint,
  endpointLabel, // 允许覆盖默认 "XXX Endpoint" 标签，兼容 S3/COS 原版带提示的文案
  regionHint,
  idLabel,
  secretLabel,
  idField = 'AccessKeyId',
  secretField = 'AccessKeySecret',
}) => {
  const k = (suffix) => `TicketAttachment${prefix}${suffix}`;
  return (
    <Row gutter={24}>
      <Col xs={24} md={8}>
        <Form.Slot label={endpointLabel || `${prefix} Endpoint`}>
          <Form.Input
            noLabel
            value={inputs[k('Endpoint')]}
            placeholder={endpointHint}
            onChange={(v) => onChange(k('Endpoint'), v)}
          />
        </Form.Slot>
      </Col>
      <Col xs={24} md={8}>
        <Form.Slot label={`${prefix} Bucket`}>
          <Form.Input
            noLabel
            value={inputs[k('Bucket')]}
            onChange={(v) => onChange(k('Bucket'), v)}
          />
        </Form.Slot>
      </Col>
      <Col xs={24} md={8}>
        <Form.Slot label={`${prefix} Region`}>
          <Form.Input
            noLabel
            value={inputs[k('Region')]}
            placeholder={regionHint}
            onChange={(v) => onChange(k('Region'), v)}
          />
        </Form.Slot>
      </Col>
      <Col xs={24} md={8}>
        <Form.Slot label={idLabel}>
          <Form.Input
            noLabel
            value={inputs[k(idField)]}
            onChange={(v) => onChange(k(idField), v)}
          />
        </Form.Slot>
      </Col>
      <Col xs={24} md={8}>
        <Form.Slot label={secretLabel}>
          <Form.Input
            noLabel
            type='password'
            value={inputs[k(secretField)]}
            placeholder={t('敏感信息不会回显')}
            onChange={(v) => onChange(k(secretField), v)}
          />
        </Form.Slot>
      </Col>
      <Col xs={24} md={8}>
        <Form.Slot label={t('自定义域名（可选）')}>
          <Form.Input
            noLabel
            value={inputs[k('CustomDomain')]}
            placeholder='https://cdn.example.com'
            onChange={(v) => onChange(k('CustomDomain'), v)}
          />
        </Form.Slot>
      </Col>
    </Row>
  );
};

// =====================================================
// 成员选择弹窗
// =====================================================
const StaffPickerModal = ({ t, visible, onCancel, onConfirm, staffList, selected, setSelected, roleLabel }) => {
  const rowSelection = {
    selectedRowKeys: selected,
    onChange: (keys) => setSelected(keys),
  };

  const columns = [
    {
      title: t('账号'),
      dataIndex: 'username',
      render: (_, r) => (
        <Space>
          <Text strong>{r.display_name || r.username}</Text>
          {r.display_name && (
            <Text type='tertiary' size='small'>
              @{r.username}
            </Text>
          )}
        </Space>
      ),
    },
    {
      title: t('角色'),
      dataIndex: 'role',
      width: 120,
      render: (role) => <Tag color='blue'>{roleLabel(role, t)}</Tag>,
    },
    {
      title: t('邮箱'),
      dataIndex: 'email',
      render: (v) => v || <Text type='tertiary'>—</Text>,
    },
  ];

  return (
    <Modal
      centered
      title={t('选择客服')}
      visible={visible}
      onCancel={onCancel}
      onOk={onConfirm}
      okText={t('确认选中')}
      cancelText={t('取消')}
      width={720}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{ maxHeight: 'calc(80vh - 120px)', overflowY: 'auto', overflowX: 'hidden' }}
    >
      {staffList.length === 0 ? (
        <Banner
          type='warning'
          fullMode={false}
          closeIcon={null}
          title={t('暂无可选客服')}
          description={t(
            '请先到"用户"页面，选择要担任客服的账号并将其角色改为"客服"或更高。',
          )}
        />
      ) : (
        <>
          <Text type='tertiary' style={{ display: 'block', marginBottom: 8 }}>
            {t('勾选后点击"确认选中"。允许一位客服同时出现在多个分组里。')}
          </Text>
          <Table
            rowKey='id'
            dataSource={staffList}
            columns={columns}
            rowSelection={rowSelection}
            pagination={false}
            size='small'
          />
        </>
      )}
    </Modal>
  );
};

export default TicketSetting;

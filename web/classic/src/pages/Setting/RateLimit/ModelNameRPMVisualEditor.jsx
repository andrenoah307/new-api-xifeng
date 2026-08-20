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

import React, { useMemo, useRef, useState } from 'react';
import {
  Button,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus, IconSearch } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers';
import {
  MODEL_NAME_RPM_MAX_GLOBAL,
  deleteModelNameRPMGroupTotalRule,
  deleteModelNameRPMRule,
  parseModelNameRPMConfig,
  upsertModelNameRPMGroupTotalRule,
  upsertModelNameRPMRule,
  validateModelNameRPMGroupTotalRule,
  validateModelNameRPMRule,
} from '../../../helpers/model-name-rpm';

const { Text } = Typography;

const ERROR_MESSAGES = {
  'model-name-required': 'Model name is required',
  'model-name-too-long': 'Model name must not exceed 255 characters',
  'model-name-whitespace':
    'Model name must not contain whitespace or control characters',
  'model-name-duplicate': 'This model already has a rule',
  'global-rpm-range':
    'Global RPM must be an integer between 0 and 1,000,000 (0 means unlimited)',
  'unlimited-without-sublimit':
    'When the global RPM is 0 (unlimited), configure at least one per-user or per-group limit; otherwise delete this model rule',
  'user-rpm-range': 'User RPM must be 0 or a positive integer',
  'user-rpm-exceeds-global': 'User RPM must not exceed the global RPM',
  'group-name-required': 'Select a group',
  'group-name-too-long': 'Group name must not exceed 64 characters',
  'group-name-whitespace':
    'Group name must not contain whitespace or control characters',
  'group-name-duplicate': 'This group already has a limit for this model',
  'group-rpm-range': 'Group RPM must be an integer greater than 0',
  'group-rpm-exceeds-global': 'Group RPM must not exceed the global RPM',
  'group-total-name-required': 'Group total name is required',
  'group-total-name-too-long': 'Group total name must not exceed 64 characters',
  'group-total-name-whitespace':
    'Group total name must not contain whitespace or control characters',
  'group-total-name-duplicate': 'This group already has a total RPM limit',
  'group-total-rpm-range':
    'Total RPM must be an integer between 0 and 1,000,000 (0 means no total limit)',
  'group-total-user-rpm-range':
    'Per-user RPM must be an integer between 0 and 1,000,000 (0 means no per-user limit)',
  'group-total-user-rpm-exceeds-total':
    'Per-user RPM must not exceed the total RPM when the total limit is enabled',
  'group-total-without-limit':
    'Total RPM and per-user RPM cannot both be 0; delete the group entry instead',
};

const MODEL_NAME_ERROR_CODES = [
  'model-name-required',
  'model-name-too-long',
  'model-name-whitespace',
  'model-name-duplicate',
];

const GROUP_ERROR_CODES = [
  'group-name-required',
  'group-name-too-long',
  'group-name-whitespace',
  'group-name-duplicate',
  'group-rpm-range',
  'group-rpm-exceeds-global',
];

export default function ModelNameRPMVisualEditor({ value, onChange }) {
  const { t } = useTranslation();

  const [searchText, setSearchText] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editingModelName, setEditingModelName] = useState(null);
  const [modelName, setModelName] = useState('');
  const [globalRpm, setGlobalRpm] = useState(60);
  const [userRpm, setUserRpm] = useState(0);
  const [groups, setGroups] = useState([]);
  const [error, setError] = useState(null);
  const [groupTotalModalVisible, setGroupTotalModalVisible] = useState(false);
  const [editingGroupName, setEditingGroupName] = useState(null);
  const [groupTotalName, setGroupTotalName] = useState('');
  const [totalRpm, setTotalRpm] = useState(30);
  const [groupUserRpm, setGroupUserRpm] = useState(0);
  const [groupTotalError, setGroupTotalError] = useState(null);
  const [groupOptions, setGroupOptions] = useState([]);
  const groupsLoadedRef = useRef(false);

  const { rules, groupTotals } = useMemo(() => {
    const parsed = parseModelNameRPMConfig(value);
    return parsed.ok
      ? { rules: parsed.rules, groupTotals: parsed.groupTotals }
      : { rules: [], groupTotals: [] };
  }, [value]);

  const filteredRules = useMemo(() => {
    const keyword = searchText.trim().toLowerCase();
    if (!keyword) return rules;
    return rules.filter((rule) =>
      rule.modelName.toLowerCase().includes(keyword),
    );
  }, [rules, searchText]);

  // /api/group/ serves an in-memory ratio snapshot and never touches the
  // database, so fetching it once per mount costs nothing beyond one request.
  async function loadGroupOptions() {
    if (groupsLoadedRef.current) return;
    groupsLoadedRef.current = true;
    try {
      const res = await API.get('/api/group/');
      if (!res.data.success) return;
      const data = res.data.data;
      setGroupOptions(
        Array.isArray(data) ? data.map(String) : Object.keys(data || {}),
      );
    } catch {
      groupsLoadedRef.current = false;
    }
  }

  function openModal(rule) {
    setError(null);
    setEditingModelName(rule ? rule.modelName : null);
    setModelName(rule ? rule.modelName : '');
    setGlobalRpm(rule ? rule.globalRpm : 60);
    setUserRpm(rule ? rule.userRpm : 0);
    setGroups(rule ? rule.groups.map((group) => ({ ...group })) : []);
    setModalVisible(true);
    loadGroupOptions();
  }

  function updateGroup(index, patch) {
    setGroups((previous) =>
      previous.map((group, current) =>
        current === index ? { ...group, ...patch } : group,
      ),
    );
  }

  function handleSave() {
    const rule = { modelName, globalRpm, userRpm, groups };
    const otherModelNames = rules
      .map((item) => item.modelName)
      .filter((name) => name !== editingModelName);

    const validationError = validateModelNameRPMRule(rule, otherModelNames);
    if (validationError) {
      setError(validationError);
      return;
    }

    onChange(upsertModelNameRPMRule(value, editingModelName, rule));
    setModalVisible(false);
  }

  function openGroupTotalModal(rule) {
    setEditingGroupName(rule ? rule.groupName : null);
    setGroupTotalName(rule ? rule.groupName : '');
    setTotalRpm(rule ? rule.totalRpm : 30);
    setGroupUserRpm(rule ? rule.userRpm : 0);
    setGroupTotalError(null);
    setGroupTotalModalVisible(true);
  }

  function handleGroupTotalSave() {
    const rule = {
      groupName: groupTotalName,
      totalRpm,
      userRpm: groupUserRpm,
    };
    const validationError = validateModelNameRPMGroupTotalRule(
      rule,
      groupTotals
        .map((item) => item.groupName)
        .filter((name) => name !== editingGroupName),
    );
    if (validationError) {
      setGroupTotalError(validationError);
      return;
    }

    onChange(upsertModelNameRPMGroupTotalRule(value, editingGroupName, rule));
    setGroupTotalModalVisible(false);
  }

  function fieldError(codes, groupIndex) {
    if (!error || !codes.includes(error.code)) return null;
    const errorGroupIndex =
      error.groupIndex === undefined ? null : error.groupIndex;
    if (groupIndex === undefined) {
      if (errorGroupIndex !== null) return null;
    } else if (errorGroupIndex !== groupIndex) {
      return null;
    }
    return (
      <Text type='danger' size='small'>
        {t(ERROR_MESSAGES[error.code])}
      </Text>
    );
  }

  function groupTotalFieldError(codes) {
    if (!groupTotalError || !codes.includes(groupTotalError.code)) return null;
    return (
      <Text type='danger' size='small'>
        {t(ERROR_MESSAGES[groupTotalError.code])}
      </Text>
    );
  }

  // A group already present in the document must stay selectable even if it was
  // removed from the group catalog, otherwise editing would silently drop it.
  const selectableGroups = Array.from(
    new Set([...groupOptions, ...groups.map((group) => group.groupName)]),
  ).filter((group) => group !== '');

  const columns = [
    {
      title: t('Model name'),
      dataIndex: 'modelName',
    },
    {
      title: t('Global RPM'),
      dataIndex: 'globalRpm',
      render: (globalRpmValue) =>
        globalRpmValue === 0 ? t('Unlimited') : globalRpmValue.toLocaleString(),
    },
    {
      title: t('每用户 RPM'),
      dataIndex: 'userRpm',
      render: (userRpmValue) =>
        userRpmValue === 0 ? t('None') : userRpmValue.toLocaleString(),
    },
    {
      title: t('Group sub-limits'),
      dataIndex: 'groups',
      render: (ruleGroups) =>
        ruleGroups.length === 0 ? (
          <Text type='tertiary' size='small'>
            {t('None')}
          </Text>
        ) : (
          <Space wrap>
            {ruleGroups.map((group) => (
              <Tag key={group.groupName} color='blue'>
                {`${group.groupName}: ${group.rpm.toLocaleString()}`}
              </Tag>
            ))}
          </Space>
        ),
    },
    {
      title: t('操作'),
      dataIndex: 'actions',
      render: (text, rule) => (
        <Space>
          <Button size='small' onClick={() => openModal(rule)}>
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('删除')}
            content={rule.modelName}
            onConfirm={() =>
              onChange(deleteModelNameRPMRule(value, rule.modelName))
            }
          >
            <Button size='small' type='danger'>
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const groupTotalColumns = [
    {
      title: t('Group name'),
      dataIndex: 'groupName',
      render: (groupName) => (
        <Space wrap>
          <Text>{groupName}</Text>
          <Tag color='blue'>{t('All models combined')}</Tag>
        </Space>
      ),
    },
    {
      title: t('Total RPM'),
      dataIndex: 'totalRpm',
      render: (totalRpmValue) =>
        totalRpmValue === 0 ? t('None') : totalRpmValue.toLocaleString(),
    },
    {
      title: t('每用户 RPM'),
      dataIndex: 'userRpm',
      render: (userRpmValue) =>
        userRpmValue === 0 ? t('None') : userRpmValue.toLocaleString(),
    },
    {
      title: t('操作'),
      dataIndex: 'actions',
      render: (text, rule) => (
        <Space>
          <Button size='small' onClick={() => openGroupTotalModal(rule)}>
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('删除')}
            content={rule.groupName}
            onConfirm={() =>
              onChange(deleteModelNameRPMGroupTotalRule(value, rule.groupName))
            }
          >
            <Button size='small' type='danger'>
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      <div style={{ marginBottom: 24 }}>
        <Space
          style={{
            marginBottom: 10,
            width: '100%',
            justifyContent: 'space-between',
            alignItems: 'flex-start',
          }}
        >
          <div>
            <Text strong>{t('Group total RPM')}</Text>
            <div>
              <Text type='tertiary' size='small'>
                {t(
                  'Top-level group limits apply to every model in the group (including models not listed in the models section): total_rpm caps all users combined, user_rpm caps a single user, and 0 means no limit.',
                )}
              </Text>
            </div>
          </div>
          <Button icon={<IconPlus />} onClick={() => openGroupTotalModal(null)}>
            {t('Add group total')}
          </Button>
        </Space>
        <Table
          columns={groupTotalColumns}
          dataSource={groupTotals}
          rowKey='groupName'
          pagination={false}
          size='small'
          empty={t('No group total RPM limits configured.')}
        />
      </div>

      <Space style={{ marginBottom: 10, width: '100%' }}>
        <Input
          prefix={<IconSearch />}
          placeholder={t('Search model names...')}
          value={searchText}
          onChange={setSearchText}
          showClear
        />
        <Button icon={<IconPlus />} onClick={() => openModal(null)}>
          {t('Add model')}
        </Button>
      </Space>

      <Table
        columns={columns}
        dataSource={filteredRules}
        rowKey='modelName'
        pagination={false}
        size='small'
        empty={
          searchText
            ? t('No models match your search')
            : t(
                'No model RPM rules configured. Click "Add model" to get started.',
              )
        }
      />

      <Modal
        title={t('Group total RPM')}
        visible={groupTotalModalVisible}
        onCancel={() => setGroupTotalModalVisible(false)}
        onOk={handleGroupTotalSave}
        okText={t('保存')}
        cancelText={t('取消')}
        centered
        width={520}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        <Text type='tertiary' size='small'>
          {t(
            'Top-level group limits apply to every model in the group (including models not listed in the models section): total_rpm caps all users combined, user_rpm caps a single user, and 0 means no limit.',
          )}
        </Text>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('Group name')}</Text>
          <Input
            value={groupTotalName}
            onChange={setGroupTotalName}
            style={{ marginTop: 4 }}
          />
          <div>
            {groupTotalFieldError([
              'group-total-name-required',
              'group-total-name-too-long',
              'group-total-name-whitespace',
              'group-total-name-duplicate',
            ])}
          </div>
        </div>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('Total RPM')}</Text>
          <InputNumber
            value={totalRpm}
            min={0}
            max={MODEL_NAME_RPM_MAX_GLOBAL}
            step={1}
            onChange={(next) => setTotalRpm(Number(next) || 0)}
            style={{ marginTop: 4, width: '100%' }}
          />
          <div>
            {groupTotalFieldError([
              'group-total-rpm-range',
              'group-total-without-limit',
            ])}
          </div>
        </div>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('每用户 RPM')}</Text>
          <InputNumber
            value={groupUserRpm}
            min={0}
            max={MODEL_NAME_RPM_MAX_GLOBAL}
            step={1}
            onChange={(next) => setGroupUserRpm(Number(next) || 0)}
            style={{ marginTop: 4, width: '100%' }}
          />
          <div>
            {groupTotalFieldError([
              'group-total-user-rpm-range',
              'group-total-user-rpm-exceeds-total',
            ])}
          </div>
        </div>
      </Modal>

      <Modal
        title={
          editingModelName === null
            ? t('Add model RPM rule')
            : t('Edit model RPM rule')
        }
        visible={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSave}
        okText={t('保存')}
        cancelText={t('取消')}
        centered
        width={640}
        style={{ maxWidth: '92vw' }}
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        <Text type='tertiary' size='small'>
          {t(
            'Requests consume the global bucket and, when configured, the per-user and matching group buckets.',
          )}
        </Text>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('Model name')}</Text>
          <Input
            value={modelName}
            placeholder={t('e.g., gpt-4o')}
            onChange={setModelName}
            style={{ marginTop: 4 }}
          />
          <div>
            <Text type='tertiary' size='small'>
              {t(
                'Must match the model name the client requests, including aliases.',
              )}
            </Text>
          </div>
          {fieldError(MODEL_NAME_ERROR_CODES)}
        </div>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('Global RPM')}</Text>
          <InputNumber
            value={globalRpm}
            min={0}
            max={MODEL_NAME_RPM_MAX_GLOBAL}
            step={1}
            onChange={(next) => setGlobalRpm(Number(next) || 0)}
            style={{ marginTop: 4, width: '100%' }}
          />
          <div>
            <Text type='tertiary' size='small'>
              {t('Hard ceiling shared by every group, in requests per minute.')}
            </Text>
          </div>
          <div>
            <Text type='tertiary' size='small'>
              {t(
                '0 means unlimited; usage is still counted but is not shown in the RPM overview.',
              )}
            </Text>
          </div>
          {fieldError(['global-rpm-range', 'unlimited-without-sublimit'])}
        </div>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('每用户 RPM')}</Text>
          <InputNumber
            value={userRpm}
            min={0}
            max={MODEL_NAME_RPM_MAX_GLOBAL}
            step={1}
            onChange={(next) => setUserRpm(Number(next) || 0)}
            style={{ marginTop: 4, width: '100%' }}
          />
          <div>
            <Text type='tertiary' size='small'>
              {t('Optional limit for each user. Set to 0 to disable.')}
            </Text>
          </div>
          {fieldError(['user-rpm-range', 'user-rpm-exceeds-global'])}
        </div>

        <div style={{ marginTop: 16 }}>
          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
            <Text strong>{t('Group sub-limits')}</Text>
            <Button
              size='small'
              icon={<IconPlus />}
              onClick={() =>
                setGroups((previous) => [
                  ...previous,
                  { groupName: '', rpm: 1 },
                ])
              }
            >
              {t('Add group limit')}
            </Button>
          </Space>

          {groups.length === 0 ? (
            <div style={{ marginTop: 8 }}>
              <Text type='tertiary' size='small'>
                {t('No group sub-limit. Only the global RPM applies.')}
              </Text>
            </div>
          ) : (
            groups.map((group, index) => (
              <div key={index} style={{ marginTop: 8 }}>
                <Space>
                  <Select
                    value={group.groupName}
                    placeholder={t('Select a group')}
                    optionList={selectableGroups.map((option) => ({
                      label: option,
                      value: option,
                    }))}
                    onChange={(next) =>
                      updateGroup(index, { groupName: String(next) })
                    }
                    getPopupContainer={() => document.body}
                    style={{ width: 240 }}
                  />
                  <InputNumber
                    value={group.rpm}
                    min={1}
                    max={MODEL_NAME_RPM_MAX_GLOBAL}
                    step={1}
                    aria-label={t('Group RPM')}
                    onChange={(next) =>
                      updateGroup(index, { rpm: Number(next) || 0 })
                    }
                    style={{ width: 140 }}
                  />
                  <Button
                    type='danger'
                    icon={<IconDelete />}
                    aria-label={t('Remove group limit')}
                    onClick={() =>
                      setGroups((previous) =>
                        previous.filter((item, current) => current !== index),
                      )
                    }
                  />
                </Space>
                <div>{fieldError(GROUP_ERROR_CODES, index)}</div>
              </div>
            ))
          )}
        </div>
      </Modal>
    </>
  );
}

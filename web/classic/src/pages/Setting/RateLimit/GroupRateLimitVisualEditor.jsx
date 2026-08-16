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
  Typography,
} from '@douyinfe/semi-ui';
import { IconPlus, IconSearch } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../../helpers';
import {
  deleteGroupRateLimitRule,
  parseGroupRateLimitConfig,
  upsertGroupRateLimitRule,
  validateGroupRateLimitRule,
} from '../../../helpers/group-rate-limit';

const { Text } = Typography;

const ERROR_MESSAGES = {
  'rule-invalid': '分组速率限制规则无效',
  'group-name-required': '请选择分组',
  'group-name-control': '组名不能包含控制字符',
  'group-name-duplicate': '该分组已有速率限制',
  'total-count-range': '总请求数必须是 0 到 2147483647 之间的整数',
  'success-count-range': '成功请求数必须是 1 到 2147483647 之间的整数',
};

function getRuleErrors(rule, rules, originalGroupName) {
  const errors = [...validateGroupRateLimitRule(rule).errors];
  let skippedOriginal = false;
  const duplicate = rules.includes(rule)
    ? rules.filter((other) => other.groupName === rule.groupName).length > 1
    : rules.some((other) => {
        if (other.groupName !== rule.groupName) return false;
        if (
          originalGroupName !== null &&
          originalGroupName !== undefined &&
          other.groupName === originalGroupName &&
          !skippedOriginal
        ) {
          skippedOriginal = true;
          return false;
        }
        return true;
      });
  if (!errors.includes('group-name-required') && duplicate) {
    errors.push('group-name-duplicate');
  }
  return errors;
}

export default function GroupRateLimitVisualEditor({ value, onChange }) {
  const { t } = useTranslation();
  const [searchText, setSearchText] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editingGroupName, setEditingGroupName] = useState(null);
  const [groupName, setGroupName] = useState('');
  const [totalCount, setTotalCount] = useState(0);
  const [successCount, setSuccessCount] = useState(1);
  const [errorCodes, setErrorCodes] = useState([]);
  const [groupOptions, setGroupOptions] = useState([]);
  const groupsLoadedRef = useRef(false);

  const parsed = useMemo(() => parseGroupRateLimitConfig(value), [value]);
  const rules = parsed.ok ? parsed.rules : [];

  const filteredRules = useMemo(() => {
    const keyword = searchText.trim().toLowerCase();
    if (!keyword) return rules;
    return rules.filter((rule) =>
      rule.groupName.toLowerCase().includes(keyword),
    );
  }, [rules, searchText]);

  async function loadGroupOptions() {
    if (groupsLoadedRef.current) return;
    groupsLoadedRef.current = true;
    try {
      const response = await API.get('/api/group/');
      if (!response.data.success) {
        groupsLoadedRef.current = false;
        return;
      }
      const data = response.data.data;
      setGroupOptions(
        Array.isArray(data) ? data.map(String) : Object.keys(data || {}),
      );
    } catch {
      groupsLoadedRef.current = false;
    }
  }

  function openModal(rule) {
    setErrorCodes([]);
    setEditingGroupName(rule ? rule.groupName : null);
    setGroupName(rule ? rule.groupName : '');
    setTotalCount(rule ? rule.totalCount : 0);
    setSuccessCount(rule ? rule.successCount : 1);
    setModalVisible(true);
    void loadGroupOptions();
  }

  function updateModalGroupName(next) {
    setGroupName(String(next));
    setErrorCodes([]);
  }

  function handleSave() {
    const rule = { groupName, totalCount, successCount };
    const errors = getRuleErrors(rule, rules, editingGroupName);
    if (errors.length > 0) {
      setErrorCodes(errors);
      return;
    }

    const result = upsertGroupRateLimitRule(value, rule, editingGroupName);
    if (!result.ok) {
      showError(t('无效的分组速率限制文档'));
      return;
    }

    onChange(result.json);
    setModalVisible(false);
  }

  function handleDelete(name) {
    const result = deleteGroupRateLimitRule(value, name);
    if (!result.ok) {
      showError(t('无效的分组速率限制文档'));
      return;
    }
    onChange(result.json);
  }

  function fieldError() {
    if (errorCodes.length === 0) return null;
    return (
      <Text type='danger' size='small'>
        {errorCodes.map((code) => t(ERROR_MESSAGES[code])).join('；')}
      </Text>
    );
  }

  const selectableGroups = useMemo(() => {
    const catalog = new Set(groupOptions);
    const allNames = Array.from(
      new Set([...groupOptions, ...rules.map((rule) => rule.groupName)]),
    );
    return allNames
      .filter(
        (name) =>
          name !== '' && (editingGroupName !== null || catalog.has(name)),
      )
      .map((name) => ({
        label: catalog.has(name) ? name : `${name} (${t('已删除')})`,
        value: name,
      }));
  }, [editingGroupName, groupOptions, rules, t]);

  if (!parsed.ok) {
    return <Text type='danger'>{t('修复 JSON 后再切换到可视化编辑器。')}</Text>;
  }

  const columns = [
    {
      title: t('组名'),
      dataIndex: 'groupName',
      render: (name, rule) => {
        const errors = getRuleErrors(rule, rules);
        return (
          <div>
            <Text type={errors.length > 0 ? 'danger' : undefined}>{name}</Text>
            {errors.length > 0 ? (
              <div>
                <Text type='danger' size='small'>
                  {errors.map((code) => t(ERROR_MESSAGES[code])).join('；')}
                </Text>
              </div>
            ) : null}
          </div>
        );
      },
    },
    {
      title: t('最多请求次数'),
      dataIndex: 'totalCount',
      render: (count, rule) => (
        <Text
          type={
            getRuleErrors(rule, rules).includes('total-count-range')
              ? 'danger'
              : undefined
          }
        >
          {count === 0 ? t('无限制') : Number(count).toLocaleString()}
        </Text>
      ),
    },
    {
      title: t('最多请求完成次数'),
      dataIndex: 'successCount',
      render: (count, rule) => (
        <Text
          type={
            getRuleErrors(rule, rules).includes('success-count-range')
              ? 'danger'
              : undefined
          }
        >
          {Number(count).toLocaleString()}
        </Text>
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
            content={rule.groupName}
            onConfirm={() => handleDelete(rule.groupName)}
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
      <Space style={{ marginBottom: 10, width: '100%' }}>
        <Input
          prefix={<IconSearch />}
          placeholder={t('搜索分组名...')}
          value={searchText}
          onChange={setSearchText}
          showClear
        />
        <Button icon={<IconPlus />} onClick={() => openModal(null)}>
          {t('添加分组')}
        </Button>
      </Space>

      <Table
        columns={columns}
        dataSource={filteredRules}
        rowKey='groupName'
        pagination={false}
        size='small'
        onRow={(rule) =>
          getRuleErrors(rule, rules).length > 0
            ? {
                style: {
                  backgroundColor: 'var(--semi-color-danger-light-default)',
                },
              }
            : {}
        }
        empty={
          searchText
            ? t('没有匹配的分组')
            : t('尚未配置分组速率限制。点击“添加分组”开始。')
        }
      />

      <Modal
        title={
          editingGroupName === null
            ? t('添加分组速率限制')
            : t('编辑分组速率限制')
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
          {t('分组速率限制按原始组名保存；目录中已不存在的组仍可编辑或删除。')}
        </Text>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('组名')}</Text>
          <Select
            value={groupName}
            placeholder={t('选择分组')}
            optionList={selectableGroups}
            onChange={updateModalGroupName}
            getPopupContainer={() => document.body}
            style={{ marginTop: 4, width: '100%' }}
          />
          <div style={{ marginTop: 4 }}>{fieldError()}</div>
        </div>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('最多请求次数')}</Text>
          <InputNumber
            value={totalCount}
            min={0}
            max={2147483647}
            step={1}
            onChange={(next) => {
              setTotalCount(Number(next));
              setErrorCodes([]);
            }}
            style={{ marginTop: 4, width: '100%' }}
          />
        </div>

        <div style={{ marginTop: 16 }}>
          <Text strong>{t('最多请求完成次数')}</Text>
          <InputNumber
            value={successCount}
            min={1}
            max={2147483647}
            step={1}
            onChange={(next) => {
              setSuccessCount(Number(next));
              setErrorCodes([]);
            }}
            style={{ marginTop: 4, width: '100%' }}
          />
        </div>
      </Modal>
    </>
  );
}

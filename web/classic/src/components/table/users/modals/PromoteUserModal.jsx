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

// 角色管理弹窗：把旧版 "提升 / 降级" 两个独立按钮合并为一个统一的角色下拉。
//   - 文件名保留 PromoteUserModal.jsx，避免上游代码大范围重命名；
//   - 实际语义改为 "RoleManagementModal"，通过调用 onChangeRole(user, targetRole) 触发 set_role 动作。
// 权限边界：
//   - 目标角色选项只显示"比当前登录管理员低"的项（RootUser 可任选）；
//   - Root 用户行在列表里不可修改角色（后端也会拦截），这里直接只读展示。

import React, { useEffect, useMemo, useState } from 'react';
import { Banner, Modal, Select, Space, Tag, Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

const ROLE_META = {
  1: { labelKey: '普通用户', color: 'blue', desc: '仅能使用个人中心与普通功能，无任何后台权限。' },
  5: { labelKey: '客服', color: 'cyan', desc: '仅能访问工单后台，根据分配规则处理指派给自己的工单。' },
  10: { labelKey: '管理员', color: 'yellow', desc: '可访问除系统设置外的全部后台功能。' },
  100: { labelKey: '超级管理员', color: 'orange', desc: '最高权限，可访问系统设置及全部功能。' },
};

const roleLabel = (role, t) => t(ROLE_META[role]?.labelKey || '未知身份');

const RoleManagementModal = ({ visible, onCancel, onConfirm, user, t, currentUserRole }) => {
  const [targetRole, setTargetRole] = useState(null);

  useEffect(() => {
    if (visible && user) {
      setTargetRole(user.role);
    }
  }, [visible, user]);

  const isRootUser = user?.role === 100;
  const myRole = Number(currentUserRole) || 1;

  const options = useMemo(() => {
    const all = [1, 5, 10, 100];
    return all
      .filter((r) => (myRole === 100 ? true : r < myRole))
      .map((r) => ({
        value: r,
        label: (
          <Space>
            <Tag color={ROLE_META[r].color} shape='circle'>
              {roleLabel(r, t)}
            </Tag>
            <Text type='tertiary' size='small'>
              {t(ROLE_META[r].desc)}
            </Text>
          </Space>
        ),
      }));
  }, [myRole, t]);

  const handleOk = () => {
    if (targetRole == null) return;
    onConfirm(targetRole);
  };

  return (
    <Modal
      centered
      title={t('管理用户角色')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleOk}
      okText={t('保存角色')}
      cancelText={t('取消')}
      okButtonProps={{ disabled: isRootUser || targetRole === user?.role }}
      width={560}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{ maxHeight: 'calc(80vh - 120px)', overflowY: 'auto', overflowX: 'hidden' }}
    >
      {isRootUser ? (
        <Banner
          type='warning'
          fullMode={false}
          closeIcon={null}
          title={t('超级管理员无法在此修改')}
          description={t('超级管理员的角色只能在数据库中手动调整，且不能被任何管理员降级。')}
        />
      ) : (
        <>
          <div style={{ marginBottom: 12 }}>
            <Text type='tertiary'>
              {t('用户：')}
              <Text strong>{user?.display_name || user?.username || '-'}</Text>
              {user?.display_name && (
                <Text type='tertiary' style={{ marginLeft: 4 }}>
                  @{user?.username}
                </Text>
              )}
              <span style={{ marginLeft: 12 }}>
                {t('当前角色：')}
                <Tag color={ROLE_META[user?.role]?.color || 'grey'} shape='circle'>
                  {roleLabel(user?.role, t)}
                </Tag>
              </span>
            </Text>
          </div>

          <div style={{ marginBottom: 8 }}>
            <Text strong>{t('目标角色')}</Text>
          </div>
          <Select
            style={{ width: '100%' }}
            value={targetRole}
            optionList={options}
            onChange={(v) => setTargetRole(v)}
            getPopupContainer={() => document.body}
            placeholder={t('请选择新的角色')}
          />

          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            style={{ marginTop: 16 }}
            title={t('角色调整说明')}
            description={
              <ul style={{ margin: '4px 0 0 16px', padding: 0 }}>
                <li>{t('你只能把用户角色修改为"比你自己低"的级别（超级管理员除外）。')}</li>
                <li>{t('设置为"客服"后，该用户可登录后台并只看到工单模块，其它管理功能都会被隐藏。')}</li>
                <li>{t('角色变更立即生效，无需重新登录；用户缓存和其名下令牌缓存都会被清空。')}</li>
              </ul>
            }
          />
        </>
      )}
    </Modal>
  );
};

export default RoleManagementModal;

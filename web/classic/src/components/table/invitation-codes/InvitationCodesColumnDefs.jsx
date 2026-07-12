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

import React from 'react';
import { Button, Dropdown, Space, Tag } from '@douyinfe/semi-ui';
import { IconMore } from '@douyinfe/semi-icons';
import { timestamp2string } from '../../../helpers';
import { INVITATION_CODE_STATUS } from './index';

export const isExpiredInvitationCode = (record) => {
  return (
    record.expired_time !== 0 &&
    record.expired_time < Math.floor(Date.now() / 1000)
  );
};

export const isExhaustedInvitationCode = (record) => {
  return record.max_uses > 0 && record.used_count >= record.max_uses;
};

const renderTimestamp = (timestamp, t) => {
  if (!timestamp || timestamp === 0) {
    return t('永不过期');
  }
  return timestamp2string(timestamp);
};

export const renderInvitationCodeStatus = (record, t) => {
  if (record.status === INVITATION_CODE_STATUS.DISABLED) {
    return (
      <Tag color='red' shape='circle'>
        {t('已禁用')}
      </Tag>
    );
  }
  if (isExpiredInvitationCode(record)) {
    return (
      <Tag color='orange' shape='circle'>
        {t('已过期')}
      </Tag>
    );
  }
  if (isExhaustedInvitationCode(record)) {
    return (
      <Tag color='violet' shape='circle'>
        {t('已用尽')}
      </Tag>
    );
  }
  return (
    <Tag color='green' shape='circle'>
      {t('生效中')}
    </Tag>
  );
};

export const getInvitationCodesColumns = ({
  t,
  copyText,
  onViewUsages,
  onEdit,
  onDelete,
  onToggleStatus,
}) => {
  return [
    {
      title: t('ID'),
      dataIndex: 'id',
      width: 72,
    },
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (text) => text || '-',
    },
    {
      title: t('邀请码'),
      dataIndex: 'code',
      render: (text) => (
        <Tag color='grey' shape='circle'>
          {text}
        </Tag>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (text, record) => renderInvitationCodeStatus(record, t),
    },
    {
      title: t('使用次数'),
      dataIndex: 'used_count',
      render: (text, record) =>
        `${record.used_count}/${record.max_uses === 0 ? t('无限') : record.max_uses}`,
    },
    {
      title: t('所有者ID'),
      dataIndex: 'owner_user_id',
      render: (text) => (text === 0 ? '-' : text),
    },
    {
      title: t('创建者ID'),
      dataIndex: 'created_by',
    },
    {
      title: t('来源'),
      dataIndex: 'is_admin',
      render: (text) => (
        <Tag color={text ? 'blue' : 'cyan'} shape='circle'>
          {text ? t('管理员') : t('用户')}
        </Tag>
      ),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      render: (text) => timestamp2string(text),
    },
    {
      title: t('过期时间'),
      dataIndex: 'expired_time',
      render: (text) => renderTimestamp(text, t),
    },
    {
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      width: 220,
      render: (text, record) => {
        const menu = [
          {
            node: 'item',
            name:
              record.status === INVITATION_CODE_STATUS.ENABLED
                ? t('禁用')
                : t('启用'),
            type:
              record.status === INVITATION_CODE_STATUS.ENABLED
                ? 'warning'
                : 'secondary',
            onClick: () =>
              onToggleStatus(
                record,
                record.status === INVITATION_CODE_STATUS.ENABLED
                  ? INVITATION_CODE_STATUS.DISABLED
                  : INVITATION_CODE_STATUS.ENABLED,
              ),
          },
          {
            node: 'item',
            name: t('删除'),
            type: 'danger',
            onClick: () => onDelete(record),
          },
        ];

        return (
          <Space>
            <Button size='small' onClick={() => copyText(record.code)}>
              {t('复制')}
            </Button>
            <Button size='small' type='tertiary' onClick={() => onViewUsages(record)}>
              {t('使用记录')}
            </Button>
            <Button size='small' type='tertiary' onClick={() => onEdit(record)}>
              {t('编辑')}
            </Button>
            <Dropdown trigger='click' position='bottomRight' menu={menu}>
              <Button type='tertiary' size='small' icon={<IconMore />} />
            </Dropdown>
          </Space>
        );
      },
    },
  ];
};

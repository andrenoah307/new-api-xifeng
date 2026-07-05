import React from 'react';
import { Button, Space, Tag, Typography } from '@douyinfe/semi-ui';
// Space 已用于操作按钮组，这里也会用于分配客服单元格中的徽章与 UID 拼接。
import { timestamp2string } from '../../../helpers';
import TicketStatusTag from '../../ticket/TicketStatusTag';
import {
  getTicketPriorityColor,
  getTicketPriorityText,
  getTicketTypeText,
} from '../../ticket/ticketUtils';

const { Text } = Typography;

// 把角色数值翻译成一个简短徽章；与对话页 resolveRoleBadge 的色阶一致，避免风格割裂。
const assigneeRoleBadge = (role, t) => {
  const r = Number(role || 0);
  if (r >= 100) return { text: t('超级管理员'), color: 'red' };
  if (r >= 10) return { text: t('管理员'), color: 'orange' };
  return { text: t('客服'), color: 'cyan' };
};

// renderAssignee 渲染"分配客服"单元格。staffIndex 是 id → TicketStaffUser 的映射。
// 缺失时退化为只显示 #id，避免因为候选列表尚未加载完就展示空白。
const renderAssignee = (assigneeId, staffIndex, t) => {
  const id = Number(assigneeId || 0);
  if (id === 0) {
    return (
      <Tag color='grey' shape='circle' size='small'>
        {t('待认领')}
      </Tag>
    );
  }
  const staff = staffIndex?.get?.(id);
  if (!staff) {
    return (
      <Text type='tertiary' size='small'>
        #{id}
      </Text>
    );
  }
  const badge = assigneeRoleBadge(staff.role, t);
  const name = staff.display_name || staff.username;
  return (
    <div className='flex flex-col'>
      <Text>{name}</Text>
      <Space spacing={4}>
        <Tag color={badge.color} shape='circle' size='small'>
          {badge.text}
        </Tag>
        <Text type='tertiary' size='small'>
          UID: {id}
        </Text>
      </Space>
    </div>
  );
};

export const getTicketsColumns = ({
  t,
  admin = false,
  onOpenDetail,
  onCloseTicket,
  staffIndex, // Map<number, TicketStaffUser>；仅在 showAssignee 时使用
  showAssignee = false, // 是否展示"分配客服"列；客服视角下工单都是自己的，这列是噪音，默认不显示
}) => {
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
      render: (value) => <Text strong>#{value}</Text>,
    },
    {
      title: t('主题'),
      dataIndex: 'subject',
      key: 'subject',
      render: (value, record) => (
        <div className='flex flex-col'>
          <Text strong>{value || '-'}</Text>
          <Text type='tertiary' size='small'>
            {getTicketTypeText(record?.type, t)}
          </Text>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (value) => <TicketStatusTag status={value} t={t} size='small' />,
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      key: 'priority',
      width: 120,
      render: (value) => (
        <Tag color={getTicketPriorityColor(value)} shape='circle' size='small'>
          {getTicketPriorityText(value, t)}
        </Tag>
      ),
    },
  ];

  if (admin) {
    columns.push({
      title: t('用户'),
      dataIndex: 'username',
      key: 'username',
      width: 140,
      render: (value, record) => (
        <div className='flex flex-col'>
          <Text>{value || '-'}</Text>
          <Text type='tertiary' size='small'>
            UID: {record?.user_id || '-'}
          </Text>
        </div>
      ),
    });
    if (showAssignee) {
      columns.push({
        title: t('分配客服'),
        dataIndex: 'assignee_id',
        key: 'assignee_id',
        width: 160,
        render: (value) => renderAssignee(value, staffIndex, t),
      });
    }
  }

  columns.push(
    {
      title: t('更新时间'),
      dataIndex: 'updated_time',
      key: 'updated_time',
      width: 180,
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
    {
      title: t('操作'),
      key: 'operate',
      width: 160,
      render: (_, record) => (
        <Space>
          <Button
            size='small'
            theme='borderless'
            type='primary'
            onClick={(event) => {
              event.stopPropagation();
              onOpenDetail?.(record);
            }}
          >
            {t('查看详情')}
          </Button>
          {!admin && Number(record?.status) !== 4 && (
            <Button
              size='small'
              theme='borderless'
              type='danger'
              onClick={(event) => {
                event.stopPropagation();
                onCloseTicket?.(record);
              }}
            >
              {t('关闭')}
            </Button>
          )}
        </Space>
      ),
    },
  );

  return columns;
};


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

import React, { useCallback, useState } from 'react';
import { Button, Card, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { API, showError, timestamp2string } from '../../helpers';
import { renderQuota } from '../../helpers/render';

const { Text } = Typography;

const CommissionRecordsCard = ({ t }) => {
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [records, setRecords] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const pageSize = 10;

  const fetchRecords = useCallback(
    async (p) => {
      setLoading(true);
      try {
        const res = await API.get(
          `/api/user/commission_records/self?page=${p}&page_size=${pageSize}`,
        );
        const { success, message, data } = res.data;
        if (success) {
          setRecords(data?.records || []);
          setTotal(data?.total || 0);
        } else {
          showError(message);
        }
      } catch {
        showError(t('请求失败'));
      } finally {
        setLoading(false);
      }
    },
    [t],
  );

  const handleExpand = () => {
    setExpanded(true);
    fetchRecords(1);
  };

  const handlePageChange = (p) => {
    setPage(p);
    fetchRecords(p);
  };

  const columns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      render: (value) => (
        <Text type='tertiary' size='small'>
          {timestamp2string(value)}
        </Text>
      ),
    },
    {
      title: t('充值金额'),
      dataIndex: 'topup_money',
      key: 'topup_money',
      render: (value) => (
        <Text size='small' style={{ fontFamily: 'monospace' }}>
          ${Number(value || 0).toFixed(2)}
        </Text>
      ),
    },
    {
      title: t('返佣比例'),
      dataIndex: 'commission_rate',
      key: 'commission_rate',
      render: (value) => <Text size='small'>{value}%</Text>,
    },
    {
      title: t('返佣金额'),
      dataIndex: 'commission_quota',
      key: 'commission_quota',
      render: (value) => (
        <Text size='small' style={{ color: 'var(--semi-color-success)', fontFamily: 'monospace' }}>
          +{renderQuota(value)}
        </Text>
      ),
    },
    {
      title: t('来源'),
      dataIndex: 'is_manual',
      key: 'is_manual',
      render: (value) => (
        <Tag color={value ? 'grey' : 'blue'} shape='circle' size='small'>
          {value ? t('手动') : t('在线')}
        </Tag>
      ),
    },
  ];

  if (!expanded) {
    return (
      <Button
        theme='borderless'
        type='tertiary'
        icon={<ChevronDown size={14} />}
        onClick={handleExpand}
        style={{ width: '100%' }}
      >
        {t('查看返佣记录')}
      </Button>
    );
  }

  return (
    <Card
      title={t('返佣记录')}
      headerExtraContent={
        <Button
          theme='borderless'
          type='tertiary'
          size='small'
          icon={<ChevronUp size={12} />}
          onClick={() => setExpanded(false)}
        >
          {t('收起')}
        </Button>
      }
    >
      <Table
        columns={columns}
        dataSource={records}
        loading={loading}
        pagination={
          total > pageSize
            ? {
                currentPage: page,
                pageSize,
                total,
                onPageChange: handlePageChange,
              }
            : false
        }
        size='small'
        rowKey='id'
        empty={
          <Text type='tertiary' size='small'>
            {t('暂无返佣记录')}
          </Text>
        }
      />
    </Card>
  );
};

export default CommissionRecordsCard;

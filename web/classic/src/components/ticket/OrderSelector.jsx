import React, { useMemo } from 'react';
import {
  Card,
  Checkbox,
  Empty,
  Space,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { timestamp2string } from '../../helpers';

const { Text } = Typography;

const OrderSelector = ({
  orders = [],
  selectedOrderIds = [],
  onChange,
  loading = false,
  t,
}) => {
  const isMobile = useIsMobile();

  const rowSelection = useMemo(
    () => ({
      selectedRowKeys: selectedOrderIds,
      onChange: (keys) => onChange?.((keys || []).map((item) => Number(item))),
    }),
    [selectedOrderIds, onChange],
  );

  const columns = useMemo(
    () => [
      {
        title: t('订单号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
      },
      {
        title: t('金额'),
        dataIndex: 'money',
        key: 'money',
        width: 120,
        render: (value) => Number(value || 0).toFixed(2),
      },
      {
        title: t('完成时间'),
        dataIndex: 'complete_time',
        key: 'complete_time',
        width: 180,
        render: (value) => (value ? timestamp2string(value) : '-'),
      },
    ],
    [t],
  );

  if (!orders.length && !loading) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={t('暂无可申请发票的充值订单')}
      />
    );
  }

  return (
    <div className='flex flex-col gap-3'>
      <Text type='secondary'>
        {t('已选择订单')}: {selectedOrderIds.length}
      </Text>
      {!isMobile ? (
        <Table
          rowKey='id'
          columns={columns}
          dataSource={orders}
          loading={loading}
          pagination={false}
          rowSelection={rowSelection}
          scroll={{ x: 'max-content' }}
        />
      ) : (
        <div className='flex flex-col gap-2'>
          {orders.map((order) => {
            const checked = selectedOrderIds.includes(Number(order.id));
            return (
              <Card key={order.id} className='!rounded-2xl shadow-sm'>
                <div className='flex items-start gap-3'>
                  <Checkbox
                    checked={checked}
                    onChange={(event) => {
                      const nextChecked = event?.target?.checked;
                      const nextIds = nextChecked
                        ? [...selectedOrderIds, Number(order.id)]
                        : selectedOrderIds.filter(
                            (item) => Number(item) !== Number(order.id),
                          );
                      onChange?.([...new Set(nextIds)]);
                    }}
                  />
                  <div className='flex-1 min-w-0'>
                    <Space vertical align='start' spacing={4}>
                      <Text strong>{order.trade_no || '-'}</Text>
                      <Text type='secondary'>
                        {t('金额')}: {Number(order.money || 0).toFixed(2)}
                      </Text>
                      <Text type='secondary'>
                        {t('完成时间')}:{' '}
                        {order.complete_time
                          ? timestamp2string(order.complete_time)
                          : '-'}
                      </Text>
                    </Space>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default OrderSelector;


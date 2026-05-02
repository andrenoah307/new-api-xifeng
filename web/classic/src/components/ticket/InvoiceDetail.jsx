import React, { useMemo } from 'react';
import { Button, Card, Descriptions, Empty, Space, Tag, Typography } from '@douyinfe/semi-ui';
import CardTable from '../common/ui/CardTable';
import { timestamp2string } from '../../helpers';
import {
  getInvoiceStatusColor,
  getInvoiceStatusText,
} from './ticketUtils';

const { Title, Text } = Typography;

const InvoiceDetail = ({
  invoice,
  orders = [],
  onStatusChange,
  loading = false,
  readonly = false,
  t,
}) => {
  const descriptionRows = useMemo(() => {
    if (!invoice) {
      return [];
    }
    return [
      { key: t('公司名称'), value: invoice.company_name || '-' },
      { key: t('税号'), value: invoice.tax_number || '-' },
      { key: t('接收邮箱'), value: invoice.email || '-' },
      { key: t('开户行'), value: invoice.bank_name || '-' },
      { key: t('银行账号'), value: invoice.bank_account || '-' },
      { key: t('注册地址'), value: invoice.company_address || '-' },
      { key: t('联系电话'), value: invoice.company_phone || '-' },
      {
        key: t('发票状态'),
        value: (
          <Tag
            color={getInvoiceStatusColor(invoice.invoice_status)}
            shape='circle'
          >
            {getInvoiceStatusText(invoice.invoice_status, t)}
          </Tag>
        ),
      },
      {
        key: t('申请金额'),
        value: Number(invoice.total_money || 0).toFixed(2),
      },
      {
        key: t('开具时间'),
        value: invoice.issued_time ? timestamp2string(invoice.issued_time) : '-',
      },
    ];
  }, [invoice, t]);

  const orderColumns = useMemo(
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

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex flex-col gap-4'>
        <div className='flex items-center justify-between gap-3'>
          <div>
            <Title heading={5} className='!mb-1'>
              {t('发票信息')}
            </Title>
            <Text type='secondary'>{t('管理员可在此查看并处理发票申请')}</Text>
          </div>
          {!readonly && invoice && typeof onStatusChange === 'function' && (
            <Space>
              <Button
                theme='light'
                type='danger'
                loading={loading}
                onClick={() => onStatusChange(3)}
              >
                {t('驳回申请')}
              </Button>
              <Button
                theme='solid'
                type='primary'
                loading={loading}
                onClick={() => onStatusChange(2)}
              >
                {t('标记已开具')}
              </Button>
            </Space>
          )}
        </div>

        {invoice ? (
          <>
            <Descriptions data={descriptionRows} />
            <div className='flex flex-col gap-2'>
              <Text strong>{t('关联订单')}</Text>
              <CardTable
                rowKey='id'
                columns={orderColumns}
                dataSource={orders}
                loading={loading}
                hidePagination
                empty={
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description={t('暂无关联订单')}
                  />
                }
              />
            </div>
          </>
        ) : (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('暂无发票信息')}
          />
        )}
      </div>
    </Card>
  );
};

export default InvoiceDetail;


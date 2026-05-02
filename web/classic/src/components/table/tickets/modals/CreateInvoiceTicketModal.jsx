import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Col,
  Empty,
  Form,
  Modal,
  Row,
  Space,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, timestamp2string } from '../../../../helpers';

const { Text, Title } = Typography;

const MIN_INVOICE_AMOUNT = 0;

const CreateInvoiceTicketModal = ({ visible, onClose, onSuccess, t }) => {
  const [loading, setLoading] = useState(false);
  const [ordersLoading, setOrdersLoading] = useState(false);
  const [orders, setOrders] = useState([]);
  const [selectedOrderIds, setSelectedOrderIds] = useState([]);
  const [invoiceAmount, setInvoiceAmount] = useState(0);
  const formApiRef = useRef(null);

  const loadOrders = async () => {
    setOrdersLoading(true);
    try {
      const res = await API.get('/api/ticket/invoice/eligible_orders');
      if (res.data?.success) {
        setOrders(res.data?.data || []);
      } else {
        showError(res.data?.message || t('加载订单失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setOrdersLoading(false);
    }
  };

  useEffect(() => {
    if (!visible) {
      setSelectedOrderIds([]);
      setOrders([]);
      setInvoiceAmount(0);
      formApiRef.current?.setValues({
        company_name: '',
        tax_number: '',
        invoice_content: '*信息技术服务*技术服务费',
        content: '',
      });
      return;
    }
    loadOrders();
  }, [visible]);

  // 选中订单时自动计算开票金额
  useEffect(() => {
    const total = orders
      .filter((o) => selectedOrderIds.includes(o.id))
      .reduce((sum, o) => sum + Number(o.money || 0), 0);
    setInvoiceAmount(Number(total.toFixed(2)));
  }, [selectedOrderIds, orders]);

  const orderColumns = useMemo(
    () => [
      {
        title: t('交易号'),
        dataIndex: 'trade_no',
        key: 'trade_no',
        ellipsis: true,
      },
      {
        title: t('支付方式'),
        dataIndex: 'payment_method',
        key: 'payment_method',
        width: 100,
        render: (v) => v || '-',
      },
      {
        title: t('实付金额'),
        dataIndex: 'money',
        key: 'money',
        width: 100,
        render: (v) => `¥ ${Number(v || 0).toFixed(2)}`,
      },
      {
        title: t('充值时间'),
        dataIndex: 'complete_time',
        key: 'complete_time',
        width: 160,
        render: (v) => (v ? timestamp2string(v) : '-'),
      },
    ],
    [t],
  );

  const rowSelection = useMemo(
    () => ({
      selectedRowKeys: selectedOrderIds,
      onChange: (keys) => setSelectedOrderIds((keys || []).map(Number)),
    }),
    [selectedOrderIds],
  );

  const canSubmit =
    selectedOrderIds.length > 0 &&
    (MIN_INVOICE_AMOUNT <= 0 || invoiceAmount >= MIN_INVOICE_AMOUNT);

  const handleSubmit = async (values) => {
    if (!selectedOrderIds.length) {
      showError(t('请选择至少一个充值订单'));
      return;
    }
    if (MIN_INVOICE_AMOUNT > 0 && invoiceAmount < MIN_INVOICE_AMOUNT) {
      showError(
        t('最低开票金额为 {{amount}} 元', { amount: MIN_INVOICE_AMOUNT }),
      );
      return;
    }

    setLoading(true);
    try {
      const res = await API.post('/api/ticket/invoice/', {
        subject: t('发票申请'),
        company_name: values.company_name,
        tax_number: values.tax_number,
        content: values.content || '',
        email: values.email || '',
        topup_order_ids: selectedOrderIds,
      });
      if (res.data?.success) {
        showSuccess(t('发票申请已提交'));
        onSuccess?.(res.data?.data);
        onClose?.();
      } else {
        showError(res.data?.message || t('发票申请提交失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={t('申请开票')}
      visible={visible}
      onCancel={onClose}
      maskClosable={false}
      width={860}
      centered
      bodyStyle={{ overflowX: 'hidden', overflowY: 'auto', maxHeight: 'calc(90vh - 120px)' }}
      footer={
        <div className='flex items-center justify-between'>
          <div>
            {MIN_INVOICE_AMOUNT > 0 && invoiceAmount < MIN_INVOICE_AMOUNT && (
              <Text type='danger' size='small'>
                {t('最低开票金额为 {{amount}} 元，当前已选金额不足', {
                  amount: MIN_INVOICE_AMOUNT,
                })}
              </Text>
            )}
          </div>
          <Space>
            <Button onClick={onClose}>{t('取消')}</Button>
            <Button
              theme='solid'
              type='primary'
              loading={loading}
              disabled={!canSubmit}
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('提交申请')}
            </Button>
          </Space>
        </div>
      }
    >
      <div className='flex flex-col gap-4'>
        {/* 第一部分：选择账单 */}
        <div>
          <Title heading={6} className='!mb-3'>
            1. {t('选择充值账单')}
          </Title>
          <Table
            rowKey='id'
            columns={orderColumns}
            dataSource={orders}
            loading={ordersLoading}
            rowSelection={rowSelection}
            pagination={false}
            size='small'
            scroll={{ y: 150 }}
            empty={
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('暂无可开票的充值订单')}
              />
            }
          />
          <div
            className='flex items-center justify-between mt-3 px-3 py-2 rounded-lg'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <Text type='secondary'>
              {t('已选择')}: {selectedOrderIds.length}/{orders.length} {t('笔')}
            </Text>
            <Space align='center'>
              <Text type='secondary'>{t('开票金额')}:</Text>
              <Text strong>¥ {invoiceAmount.toFixed(2)}</Text>
            </Space>
          </div>
        </div>

        {/* 第二部分：填写发票抬头 */}
        <div>
          <Title heading={6} className='!mb-3'>
            2. {t('填写发票抬头')}
          </Title>
          <Form
            initValues={{
              company_name: '',
              tax_number: '',
              email: '',
              invoice_content: '*信息技术服务*技术服务费',
              content: '',
            }}
            getFormApi={(api) => {
              formApiRef.current = api;
            }}
            onSubmit={handleSubmit}
            labelPosition='top'
          >
            <Row gutter={16}>
              <Col span={12}>
                <Form.Input
                  field='company_name'
                  label={t('单位名称')}
                  placeholder={t('请输入单位全称')}
                  showClear
                  rules={[
                    { required: true, message: t('单位名称不能为空') },
                  ]}
                />
              </Col>
              <Col span={12}>
                <Form.Input
                  field='tax_number'
                  label={t('纳税人识别号')}
                  placeholder={t('请输入18位统一社会信用代码')}
                  showClear
                  rules={[
                    { required: true, message: t('纳税人识别号不能为空') },
                  ]}
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Input
                  field='invoice_content'
                  label={t('发票内容')}
                  disabled
                />
              </Col>
              <Col span={12}>
                <Form.Input
                  field='email'
                  label={t('接收邮箱')}
                  placeholder={t('接收电子发票的邮箱')}
                  showClear
                />
              </Col>
            </Row>
            <Form.Input
              field='content'
              label={t('开票备注')}
              placeholder={t('请简要说明用途')}
              maxLength={100}
              showClear
            />
          </Form>
        </div>
      </div>
    </Modal>
  );
};

export default CreateInvoiceTicketModal;

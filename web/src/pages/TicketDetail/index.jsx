import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, Descriptions, Empty, Modal, Space, Tag, Typography } from '@douyinfe/semi-ui';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import TicketConversation from '../../components/ticket/TicketConversation';
import TicketReplyBox from '../../components/ticket/TicketReplyBox';
import TicketStatusTag from '../../components/ticket/TicketStatusTag';
import InvoiceDetail from '../../components/ticket/InvoiceDetail';
import RefundDetail from '../../components/ticket/RefundDetail';
import {
  canCloseTicket,
  canReplyTicket,
  getTicketPriorityColor,
  getTicketPriorityText,
  getTicketTypeText,
} from '../../components/ticket/ticketUtils';

const { Title, Text } = Typography;

const TicketDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [ticket, setTicket] = useState(null);
  const [messages, setMessages] = useState([]);
  const [invoice, setInvoice] = useState(null);
  const [invoiceOrders, setInvoiceOrders] = useState([]);
  const [refund, setRefund] = useState(null);

  const loadDetail = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/ticket/self/${id}`);
      if (res.data?.success) {
        const data = res.data?.data || {};
        setTicket(data.ticket || null);
        setMessages(data.messages || []);
        setInvoice(data.invoice || null);
        setInvoiceOrders(data.invoice_orders || []);
        setRefund(data.refund || null);
      } else {
        showError(res.data?.message || t('工单详情加载失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    loadDetail();
  }, [loadDetail]);

  const detailRows = useMemo(() => {
    if (!ticket) return [];
    return [
      { key: 'ID', value: `#${ticket.id}` },
      { key: t('工单类型'), value: getTicketTypeText(ticket.type, t) },
      {
        key: t('优先级'),
        value: (
          <Tag color={getTicketPriorityColor(ticket.priority)} shape='circle'>
            {getTicketPriorityText(ticket.priority, t)}
          </Tag>
        ),
      },
      {
        key: t('创建时间'),
        value: ticket.created_time ? timestamp2string(ticket.created_time) : '-',
      },
      {
        key: t('更新时间'),
        value: ticket.updated_time ? timestamp2string(ticket.updated_time) : '-',
      },
    ];
  }, [ticket, t]);

  const handleReply = async (content, attachmentIds = []) => {
    try {
      const res = await API.post(`/api/ticket/self/${id}/message`, {
        content,
        attachment_ids: attachmentIds,
      });
      if (res.data?.success) {
        showSuccess(t('回复已发送'));
        await loadDetail();
        return true;
      }
      showError(res.data?.message || t('回复发送失败'));
    } catch (error) {
      showError(t('请求失败'));
    }
    return false;
  };

  const handleCloseTicket = () => {
    Modal.confirm({
      title: t('确认关闭工单'),
      content: t('关闭后仍可查看历史消息，如需继续处理可联系管理员重新打开。'),
      centered: true,
      onOk: async () => {
        try {
          const res = await API.put(`/api/ticket/self/${id}/close`);
          if (res.data?.success) {
            showSuccess(t('工单已关闭'));
            loadDetail();
          } else {
            showError(res.data?.message || t('关闭工单失败'));
          }
        } catch (error) {
          showError(t('请求失败'));
        }
      },
    });
  };

  if (!ticket && !loading) {
    return (
      <div className='mt-[60px] px-2'>
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('未找到工单')}
        />
      </div>
    );
  }

  return (
    <div className='mt-[60px] px-2'>
    <div className='flex flex-col gap-4'>
      <Card className='!rounded-2xl shadow-sm border-0'>
        <div className='flex flex-col gap-4'>
          <div className='flex flex-col md:flex-row md:items-start md:justify-between gap-3'>
            <div className='flex flex-col gap-2'>
              <Space>
                <Button
                  theme='borderless'
                  onClick={() => {
                    const query = searchParams.toString();
                    navigate(`/console/ticket${query ? `?${query}` : ''}`);
                  }}
                >
                  {t('返回工单列表')}
                </Button>
                <TicketStatusTag status={ticket?.status} t={t} />
              </Space>
              <Title heading={4} className='!mb-0'>
                {ticket?.subject || '-'}
              </Title>
              <Text type='secondary'>
                {t('你可以在这里持续补充信息，管理员回复后也会保留在同一条工单中')}
              </Text>
            </div>
            {canCloseTicket(ticket) && (
              <Button theme='light' type='danger' onClick={handleCloseTicket}>
                {t('关闭工单')}
              </Button>
            )}
          </div>
          <Descriptions data={detailRows} />
        </div>
      </Card>

      {invoice && (
        <InvoiceDetail
          invoice={invoice}
          orders={invoiceOrders}
          readonly
          t={t}
        />
      )}

      {refund && (
        <RefundDetail
          refund={refund}
          ticket={ticket}
          readonly
          t={t}
        />
      )}

      <TicketConversation
        messages={messages}
        currentUserId={ticket?.user_id}
        loading={loading}
        t={t}
      />

      <TicketReplyBox
        disabled={!canReplyTicket(ticket)}
        loading={loading}
        onSubmit={handleReply}
        t={t}
      />
    </div>
    </div>
  );
};

export default TicketDetail;


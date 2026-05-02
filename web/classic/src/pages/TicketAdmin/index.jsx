import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  RadioGroup,
  Radio,
  Select,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API, isAdmin, showError, showSuccess, timestamp2string } from '../../helpers';
import { useTableCompactMode } from '../../hooks/common/useTableCompactMode';
import TicketsPage from '../../components/table/tickets';
import TicketConversation from '../../components/ticket/TicketConversation';
import TicketReplyBox from '../../components/ticket/TicketReplyBox';
import TicketStatusTag from '../../components/ticket/TicketStatusTag';
import InvoiceDetail from '../../components/ticket/InvoiceDetail';
import RefundDetail from '../../components/ticket/RefundDetail';
import TicketUserProfilePanel from '../../components/ticket/TicketUserProfilePanel';
import {
  canReplyTicket,
  getTicketPriorityColor,
  getTicketPriorityOptions,
  getTicketPriorityText,
  getTicketStatusOptions,
  getTicketTypeOptions,
  getTicketTypeText,
} from '../../components/ticket/ticketUtils';

const { Title, Text } = Typography;

// 读取当前登录账号的 id，用于判断"这张工单是不是分配给我的"以及按钮的显示逻辑。
// 与 isAdmin() 一样直接走 localStorage，不依赖 UserContext 以减少耦合。
const getCurrentAccount = () => {
  try {
    const raw = localStorage.getItem('user');
    if (!raw) return { id: 0, role: 0 };
    const u = JSON.parse(raw);
    return { id: Number(u?.id) || 0, role: Number(u?.role) || 0 };
  } catch {
    return { id: 0, role: 0 };
  }
};

const AdminTicketDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [ticket, setTicket] = useState(null);
  const [messages, setMessages] = useState([]);
  const [invoice, setInvoice] = useState(null);
  const [orders, setOrders] = useState([]);
  const [refund, setRefund] = useState(null);
  const [statusValue, setStatusValue] = useState(1);
  const [priorityValue, setPriorityValue] = useState(2);
  const account = useMemo(() => getCurrentAccount(), []);

  const loadInvoiceDetail = useCallback(async () => {
    const res = await API.get(`/api/ticket/admin/${id}/invoice`);
    if (!res.data?.success) {
      throw new Error(res.data?.message || t('发票详情加载失败'));
    }
    const data = res.data?.data || {};
    setInvoice(data.invoice || null);
    setOrders(data.orders || []);
  }, [id, t]);

  const loadRefundDetail = useCallback(async () => {
    const res = await API.get(`/api/ticket/admin/${id}/refund`);
    if (!res.data?.success) {
      throw new Error(res.data?.message || t('退款详情加载失败'));
    }
    const data = res.data?.data || {};
    setRefund(data.refund || null);
  }, [id, t]);

  const loadDetail = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/ticket/admin/${id}`);
      if (!res.data?.success) {
        throw new Error(res.data?.message || t('工单详情加载失败'));
      }
      const data = res.data?.data || {};
      setTicket(data.ticket || null);
      setMessages(data.messages || []);
      setStatusValue(Number(data.ticket?.status || 1));
      setPriorityValue(Number(data.ticket?.priority || 2));

      if (data.ticket?.type === 'invoice') {
        await loadInvoiceDetail();
      } else {
        setInvoice(null);
        setOrders([]);
      }
      if (data.ticket?.type === 'refund') {
        await loadRefundDetail();
      } else {
        setRefund(null);
      }
    } catch (error) {
      showError(error?.message || t('请求失败'));
    } finally {
      setLoading(false);
    }
  }, [id, loadInvoiceDetail, loadRefundDetail, t]);

  useEffect(() => {
    loadDetail();
  }, [loadDetail]);

  const detailRows = useMemo(() => {
    if (!ticket) return [];
    return [
      { key: 'ID', value: `#${ticket.id}` },
      { key: t('用户'), value: `${ticket.username || '-'} (UID: ${ticket.user_id || '-'})` },
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
      const res = await API.post(`/api/ticket/admin/${id}/message`, {
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

  const sendSystemMessage = async (content) => {
    const text = String(content || '').trim();
    if (!text) return false;
    try {
      const res = await API.post(`/api/ticket/admin/${id}/message`, {
        content: text,
      });
      return Boolean(res.data?.success);
    } catch (error) {
      return false;
    }
  };

  // 显式"认领"——把工单分配给当前登录用户。
  // 后端 /:id/assign 使用乐观锁：如果此时已被其他同事抢先认领，会返回 ErrTicketAssigneeInvalid，
  // 这里直接把错误消息透给用户，让他们再刷新一次即可。
  const handleClaim = async () => {
    setSaving(true);
    try {
      const res = await API.put(`/api/ticket/admin/${id}/assign`, {
        assignee_id: account.id,
        expected_assignee_id: 0,
      });
      if (res.data?.success) {
        showSuccess(t('已认领该工单'));
        await loadDetail();
      } else {
        showError(res.data?.message || t('认领失败，可能已被他人认领'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setSaving(false);
    }
  };

  const handleSaveStatus = async () => {
    setSaving(true);
    try {
      const res = await API.put(`/api/ticket/admin/${id}/status`, {
        status: statusValue,
        priority: priorityValue,
      });
      if (res.data?.success) {
        showSuccess(t('工单状态已更新'));
        await loadDetail();
      } else {
        showError(res.data?.message || t('更新工单状态失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setSaving(false);
    }
  };

  const handleRefundStatusChange = async (refundStatus, extra = {}) => {
    setSaving(true);
    try {
      const payload = { refund_status: refundStatus, ...extra };
      const res = await API.put(
        `/api/ticket/admin/${id}/refund/status`,
        payload,
      );
      if (res.data?.success) {
        showSuccess(t('退款状态已更新'));
        await loadDetail();
      } else {
        showError(res.data?.message || t('更新退款状态失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setSaving(false);
    }
  };

  const handleInvoiceStatusChange = async (invoiceStatus) => {
    setSaving(true);
    try {
      const res = await API.put(`/api/ticket/admin/${id}/invoice/status`, {
        invoice_status: invoiceStatus,
      });
      if (res.data?.success) {
        showSuccess(t('发票状态已更新'));
        await loadDetail();
      } else {
        showError(res.data?.message || t('更新发票状态失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setSaving(false);
    }
  };

  if (!ticket && !loading) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={t('未找到工单')}
      />
    );
  }

  return (
    <div className='flex flex-col gap-4'>
      <Card className='!rounded-2xl shadow-sm border-0'>
        <div className='flex flex-col gap-4'>
          <div className='flex flex-col md:flex-row md:items-start md:justify-between gap-3'>
            <div className='flex flex-col gap-2'>
              <Space wrap>
                <Button
                  theme='borderless'
                  onClick={() => {
                    const query = searchParams.toString();
                    navigate(
                      `/console/ticket_admin${query ? `?${query}` : ''}`,
                    );
                  }}
                >
                  {t('返回工单管理')}
                </Button>
                <TicketStatusTag status={ticket?.status} t={t} />
                {Number(ticket?.assignee_id || 0) === 0 ? (
                  <Tag color='grey' shape='circle'>
                    {t('待认领')}
                  </Tag>
                ) : Number(ticket?.assignee_id) === account.id ? (
                  <Tag color='green' shape='circle'>
                    {t('分配给我')}
                  </Tag>
                ) : (
                  <Tag color='blue' shape='circle'>
                    {t('处理中 · 客服 #') + ticket?.assignee_id}
                  </Tag>
                )}
                {/* 未分配时显示 "认领" 入口；回复区首条消息也会隐式认领，这里是显式快捷通道。 */}
                {Number(ticket?.assignee_id || 0) === 0 && account.id > 0 && (
                  <Button
                    size='small'
                    theme='solid'
                    type='primary'
                    loading={saving}
                    onClick={handleClaim}
                  >
                    {t('认领工单')}
                  </Button>
                )}
              </Space>
              <Title heading={4} className='!mb-0'>
                {ticket?.subject || '-'}
              </Title>
              <Text type='secondary'>
                {t('在这里回复用户、调整状态与优先级，并查看发票申请详情')}
              </Text>
            </div>
            <Space wrap>
              {ticket?.user_id ? (
                <TicketUserProfilePanel
                  ticketId={id}
                  username={ticket?.username}
                  userId={ticket?.user_id}
                  t={t}
                />
              ) : null}
              <Select
                value={statusValue}
                style={{ width: 160 }}
                optionList={getTicketStatusOptions(t)}
                onChange={setStatusValue}
              />
              <Select
                value={priorityValue}
                style={{ width: 160 }}
                optionList={getTicketPriorityOptions(t)}
                onChange={setPriorityValue}
              />
              <Button
                theme='solid'
                type='primary'
                loading={saving}
                onClick={handleSaveStatus}
              >
                {t('保存状态')}
              </Button>
            </Space>
          </div>
          <Descriptions data={detailRows} />
        </div>
      </Card>

      {ticket?.type === 'invoice' && (
        <InvoiceDetail
          invoice={invoice}
          orders={orders}
          loading={saving}
          onStatusChange={handleInvoiceStatusChange}
          t={t}
        />
      )}

      {ticket?.type === 'refund' && (
        <RefundDetail
          refund={refund}
          ticket={ticket}
          loading={saving}
          onStatusChange={handleRefundStatusChange}
          onSendMessage={sendSystemMessage}
          t={t}
        />
      )}

      <TicketConversation
        messages={messages}
        currentUserId={account.id}
        loading={loading}
        t={t}
      />

      <TicketReplyBox
        title={t('管理员回复')}
        disabled={!canReplyTicket(ticket)}
        loading={saving}
        onSubmit={handleReply}
        t={t}
      />
    </div>
  );
};

const TicketAdmin = () => {
  const { id } = useParams();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [compactMode, setCompactMode] = useTableCompactMode('tickets-admin');
  const [tickets, setTickets] = useState([]);
  const [ticketCount, setTicketCount] = useState(0);
  const [loading, setLoading] = useState(false);
  // 客服/管理员候选列表，用于把工单的 assignee_id 解析成可读姓名 + 角色徽章。
  // 只拉一次：列表相对稳定，变动频率低于工单本身。
  const [staffList, setStaffList] = useState([]);
  const activePage = Math.max(1, Number(searchParams.get('p')) || 1);
  const pageSize = Math.max(1, Number(searchParams.get('page_size')) || 10);
  const statusFilter = searchParams.get('status') || '';
  const typeFilter = searchParams.get('type') || '';
  const searchKeyword = searchParams.get('keyword') || '';
  const companyNameParam = searchParams.get('company_name') || '';
  // scope 控制视角：'' = 全部（仅管理员）/ 'mine' = 分配给我的 / 'unassigned' = 待认领池。
  // 客服默认落到 'mine'，避免打开页面就看到一堆不归自己管的工单。
  const viewerIsAdmin = isAdmin();
  const scopeParam = searchParams.get('scope') || (viewerIsAdmin ? '' : 'mine');
  const [keyword, setKeyword] = useState(searchKeyword);
  const [companyName, setCompanyName] = useState(companyNameParam);

  const updateSearchParams = useCallback(
    (patch) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          Object.entries(patch).forEach(([key, value]) => {
            if (value === undefined || value === null || value === '') {
              next.delete(key);
            } else {
              next.set(key, String(value));
            }
          });
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const setActivePage = useCallback(
    (page) => updateSearchParams({ p: page === 1 ? '' : page }),
    [updateSearchParams],
  );
  const setPageSize = useCallback(
    (size) => updateSearchParams({ page_size: size === 10 ? '' : size, p: '' }),
    [updateSearchParams],
  );
  const setStatusFilter = useCallback(
    (value) => updateSearchParams({ status: value, p: '' }),
    [updateSearchParams],
  );
  const setTypeFilter = useCallback(
    (value) => updateSearchParams({ type: value, p: '' }),
    [updateSearchParams],
  );
  const setSearchKeyword = useCallback(
    (value) => updateSearchParams({ keyword: value, p: '' }),
    [updateSearchParams],
  );
  const setCompanyNameFilter = useCallback(
    (value) => updateSearchParams({ company_name: value, p: '' }),
    [updateSearchParams],
  );
  const setScope = useCallback(
    (value) =>
      updateSearchParams({
        // 'all' 对应内部空值（管理员默认），前端 URL 保持简洁。
        scope: value === 'all' ? '' : value,
        p: '',
      }),
    [updateSearchParams],
  );

  useEffect(() => {
    setKeyword(searchKeyword);
  }, [searchKeyword]);
  useEffect(() => {
    setCompanyName(companyNameParam);
  }, [companyNameParam]);

  const loadTickets = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/ticket/admin', {
        params: {
          p: activePage,
          page_size: pageSize,
          status: statusFilter || undefined,
          type: typeFilter || undefined,
          keyword: searchKeyword || undefined,
          company_name: companyNameParam || undefined,
          scope: scopeParam || undefined,
        },
      });
      if (res.data?.success) {
        const pageData = res.data?.data || {};
        setTickets(pageData.items || []);
        setTicketCount(Number(pageData.total || 0));
      } else {
        showError(res.data?.message || t('工单列表加载失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  }, [activePage, pageSize, searchKeyword, companyNameParam, statusFilter, typeFilter, scopeParam, t]);

  useEffect(() => {
    if (id) return;
    loadTickets();
  }, [id, loadTickets]);

  // 列表页挂载时拉一次客服列表，用来解析"分配客服"列。
  // 接口对客服角色也放行（TicketStaffAuth），所以两种身份都能拿到。
  useEffect(() => {
    if (id) return;
    let ignore = false;
    (async () => {
      try {
        const res = await API.get('/api/ticket/admin/staff');
        if (!ignore && res.data?.success) {
          setStaffList(res.data.data || []);
        }
      } catch (e) {
        // 列表拉失败时，"分配客服"列会退化为只显示 #id，不影响主流程。
      }
    })();
    return () => {
      ignore = true;
    };
  }, [id]);

  const staffIndex = useMemo(() => {
    const m = new Map();
    staffList.forEach((u) => m.set(Number(u.id), u));
    return m;
  }, [staffList]);

  const statusOptions = useMemo(
    () => [
      { label: t('全部状态'), value: '' },
      ...getTicketStatusOptions(t),
    ],
    [t],
  );

  const typeOptions = useMemo(
    () => [
      { label: t('全部类型'), value: '' },
      ...getTicketTypeOptions(t, { includeInvoice: true }),
    ],
    [t],
  );

  if (id) {
    return (
      <div className='mt-[60px] px-2'>
        <AdminTicketDetail />
      </div>
    );
  }

  return (
    <div className='mt-[60px] px-2'>
    <TicketsPage
      title={t('工单管理')}
      description={t('统一查看全部工单、回复用户并推进处理状态')}
      compactMode={compactMode}
      setCompactMode={setCompactMode}
      tickets={tickets}
      loading={loading}
      activePage={activePage}
      pageSize={pageSize}
      ticketCount={ticketCount}
      handlePageChange={setActivePage}
      handlePageSizeChange={setPageSize}
      admin
      // 只有真正的管理员/超级管理员需要知道"这张工单归谁"；
      // 客服视角下工单都是分配给自己的，这一列对他们是噪音。
      showAssignee={viewerIsAdmin}
      staffIndex={staffIndex}
      onOpenDetail={(ticket) => {
        const query = searchParams.toString();
        navigate(
          `/console/ticket_admin/${ticket.id}${query ? `?${query}` : ''}`,
        );
      }}
      t={t}
      actionsArea={
        <div className='flex flex-col gap-3 w-full'>
          {/* 视角切换：管理员 3 个档位，客服 2 个档位（没有"全部"入口）。
              URL 上用 scope 参数持久化，方便刷新 / 分享链接。 */}
          <RadioGroup
            type='button'
            value={scopeParam || (viewerIsAdmin ? 'all' : 'mine')}
            onChange={(e) => setScope(e.target.value)}
          >
            {viewerIsAdmin && <Radio value='all'>{t('全部工单')}</Radio>}
            <Radio value='mine'>{t('我的工单')}</Radio>
            <Radio value='unassigned'>{t('待认领池')}</Radio>
          </RadioGroup>
          <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
            <Space wrap>
              <Input
                value={keyword}
                placeholder={t('搜索工单主题、用户名或 ID')}
                style={{ width: 280 }}
                prefix={<IconSearch />}
                showClear
                onChange={setKeyword}
                onEnterPress={() => setSearchKeyword(keyword)}
              />
              {(typeFilter === '' || typeFilter === 'invoice') && (
                <Input
                  value={companyName}
                  placeholder={t('发票抬头（公司名称）')}
                  style={{ width: 220 }}
                  prefix={<IconSearch />}
                  showClear
                  onChange={setCompanyName}
                  onEnterPress={() => setCompanyNameFilter(companyName)}
                />
              )}
            </Space>
            <Space wrap>
              <Select
                value={statusFilter}
                optionList={statusOptions}
                style={{ width: 160 }}
                onChange={setStatusFilter}
              />
              <Select
                value={typeFilter}
                optionList={typeOptions}
                style={{ width: 160 }}
                onChange={(value) => {
                  // 切到非发票类型时清理抬头搜索，避免残留参数不生效造成用户困惑。
                  if (value && value !== 'invoice' && companyNameParam) {
                    setCompanyNameFilter('');
                  }
                  setTypeFilter(value);
                }}
              />
            </Space>
          </div>
        </div>
      }
    />
    </div>
  );
};

export default TicketAdmin;


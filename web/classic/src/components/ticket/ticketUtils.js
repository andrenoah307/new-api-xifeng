export const getTicketStatusText = (status, t) => {
  switch (Number(status)) {
    case 1:
      return t('待处理');
    case 2:
      return t('处理中');
    case 3:
      return t('已解决');
    case 4:
      return t('已关闭');
    default:
      return t('未知状态');
  }
};

export const getTicketStatusColor = (status) => {
  switch (Number(status)) {
    case 1:
      return 'blue';
    case 2:
      return 'orange';
    case 3:
      return 'green';
    case 4:
      return 'grey';
    default:
      return 'grey';
  }
};

export const getTicketTypeText = (type, t) => {
  switch (type) {
    case 'refund':
      return t('退款工单');
    case 'invoice':
      return t('发票申请');
    case 'general':
    default:
      return t('普通工单');
  }
};

export const getTicketPriorityText = (priority, t) => {
  switch (Number(priority)) {
    case 1:
      return t('高优先级');
    case 3:
      return t('低优先级');
    case 2:
    default:
      return t('普通优先级');
  }
};

export const getTicketPriorityColor = (priority) => {
  switch (Number(priority)) {
    case 1:
      return 'red';
    case 3:
      return 'grey';
    case 2:
    default:
      return 'blue';
  }
};

export const getInvoiceStatusText = (status, t) => {
  switch (Number(status)) {
    case 1:
      return t('待开具');
    case 2:
      return t('已开具');
    case 3:
      return t('已驳回');
    default:
      return t('未知状态');
  }
};

export const getInvoiceStatusColor = (status) => {
  switch (Number(status)) {
    case 1:
      return 'orange';
    case 2:
      return 'green';
    case 3:
      return 'red';
    default:
      return 'grey';
  }
};

export const getTicketTypeOptions = (t, { includeInvoice = false } = {}) => {
  const options = [
    { label: t('普通工单'), value: 'general' },
    { label: t('退款工单'), value: 'refund' },
  ];
  if (includeInvoice) {
    options.push({ label: t('发票申请'), value: 'invoice' });
  }
  return options;
};

export const getTicketPriorityOptions = (t) => [
  { label: t('高优先级'), value: 1 },
  { label: t('普通优先级'), value: 2 },
  { label: t('低优先级'), value: 3 },
];

export const getTicketStatusOptions = (t, { allowClosed = true } = {}) => {
  const options = [
    { label: t('待处理'), value: 1 },
    { label: t('处理中'), value: 2 },
    { label: t('已解决'), value: 3 },
  ];
  if (allowClosed) {
    options.push({ label: t('已关闭'), value: 4 });
  }
  return options;
};

export const canReplyTicket = (ticket) => Number(ticket?.status) !== 4;
export const canCloseTicket = (ticket) => Number(ticket?.status) !== 4;

export const getRefundStatusText = (status, t) => {
  switch (Number(status)) {
    case 1:
      return t('待审核');
    case 2:
      return t('已退款');
    case 3:
      return t('已驳回');
    default:
      return t('未知状态');
  }
};

export const getRefundStatusColor = (status) => {
  switch (Number(status)) {
    case 1:
      return 'orange';
    case 2:
      return 'green';
    case 3:
      return 'red';
    default:
      return 'grey';
  }
};

const REFUND_PAYEE_TYPES = [
  { value: 'alipay', labelKey: '支付宝' },
  { value: 'wechat', labelKey: '微信' },
  { value: 'bank', labelKey: '银行卡' },
  { value: 'other', labelKey: '其他' },
];

export const getRefundPayeeTypeText = (payeeType, t) => {
  const item = REFUND_PAYEE_TYPES.find(
    (x) => x.value === String(payeeType || '').toLowerCase(),
  );
  return item ? t(item.labelKey) : '-';
};

export const getRefundPayeeTypeOptions = (t) =>
  REFUND_PAYEE_TYPES.map((x) => ({ label: t(x.labelKey), value: x.value }));


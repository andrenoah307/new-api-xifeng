import React, { useEffect, useRef, useState } from 'react';
import {
  ArrowLeft,
  ChevronRight,
  MessageSquare,
  Receipt,
  Wallet,
} from 'lucide-react';
import CreateInvoiceTicketModal from './CreateInvoiceTicketModal';
import RefundFinanceDetail from '../../../ticket/RefundFinanceDetail';
import {
  Banner,
  Button,
  Checkbox,
  Form,
  Modal,
  Typography,
  Upload,
} from '@douyinfe/semi-ui';
import { IconUpload } from '@douyinfe/semi-icons';
import {
  API,
  getCurrencyConfig,
  renderQuotaWithAmount,
  showError,
  showSuccess,
} from '../../../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../../../helpers/quota';
import {
  getRefundPayeeTypeOptions,
  getTicketPriorityOptions,
} from '../../../ticket/ticketUtils';
import { useTicketAttachments } from '../../../ticket/useTicketAttachments';

const { Text } = Typography;

const CreateTicketModal = ({ visible, onClose, onSuccess, t }) => {
  const [step, setStep] = useState('choose');
  const [loading, setLoading] = useState(false);
  const [ticketType, setTicketType] = useState('general');
  const [payeeType, setPayeeType] = useState('alipay');
  const [userQuota, setUserQuota] = useState(0);
  const [maxRefundableQuota, setMaxRefundableQuota] = useState(0);
  const [requestableQuota, setRequestableQuota] = useState(0);
  const [eligibility, setEligibility] = useState(null);
  const [eligibilityError, setEligibilityError] = useState(false);
  const [quotaLoading, setQuotaLoading] = useState(false);
  const [invoiceConflict, setInvoiceConflict] = useState(null);
  const [invoiceConflictAcked, setInvoiceConflictAcked] = useState(false);
  const [limitStatus, setLimitStatus] = useState(null);
  const formApiRef = useRef(null);
  const refundRequest = useRef({ signature: '', key: '' });
  const [refundAmount, setRefundAmount] = useState('');
  const [financeQuote, setFinanceQuote] = useState(null);
  const [quoteError, setQuoteError] = useState('');
  const [quoteLoading, setQuoteLoading] = useState(false);
  const [quoteRevision, setQuoteRevision] = useState(0);
  useEffect(() => {
    setFinanceQuote(null);
    setQuoteError('');
    const quota = displayAmountToQuota(Number(refundAmount) || 0);
    if (
      !visible ||
      step !== 'details' ||
      ticketType !== 'refund' ||
      !Number.isFinite(quota) ||
      quota <= 0
    ) {
      setQuoteLoading(false);
      return;
    }
    setQuoteLoading(true);
    const controller = new AbortController();
    const timer = setTimeout(async () => {
      try {
        const response = await API.get('/api/ticket/refund/finance-quote', {
          params: { refund_quota: quota },
          signal: controller.signal,
          disableDuplicate: true,
        });
        if (!response.data.success) throw new Error(response.data.message);
        if (!controller.signal.aborted) setFinanceQuote(response.data.data);
      } catch (failure) {
        if (!controller.signal.aborted) setQuoteError(failure.message);
      } finally {
        if (!controller.signal.aborted) setQuoteLoading(false);
      }
    }, 500);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [visible, step, ticketType, refundAmount, quoteRevision]);
  const {
    config,
    attachments,
    uploading,
    reset,
    discardAll,
    uploadProps,
    uploadRef,
    handlePaste,
  } = useTicketAttachments(t);

  const loadUserQuota = async () => {
    setQuotaLoading(true);
    setEligibility(null);
    setEligibilityError(false);
    setMaxRefundableQuota(0);
    setRequestableQuota(0);
    try {
      const res = await API.get('/api/ticket/refund/eligibility');
      if (res.data?.success) {
        setEligibility(res.data.data);
        setUserQuota(Number(res.data?.data?.wallet_quota || 0));
        setMaxRefundableQuota(
          Number(res.data?.data?.max_refundable_quota ?? 0),
        );
        setRequestableQuota(Number(res.data?.data?.requestable_quota ?? 0));
      } else setEligibilityError(true);
    } catch (error) {
      setEligibilityError(true);
    } finally {
      setQuotaLoading(false);
    }
  };

  const loadLimitStatus = async () => {
    try {
      const res = await API.get('/api/ticket/limit-status', {
        skipErrorHandler: true,
      });
      if (res.data?.success) {
        setLimitStatus(res.data?.data || null);
      }
    } catch (error) {}
  };

  useEffect(() => {
    if (!visible) {
      setStep('choose');
      refundRequest.current = { signature: '', key: '' };
      setRefundAmount('');
      setTicketType('general');
      setPayeeType('alipay');
      setLimitStatus(null);
      // Modal 关闭时清空本地选中附件（真实删除交给父级的 cancel 流程处理）。
      reset();
      return;
    }
    loadLimitStatus();
    formApiRef.current?.setValues({
      type: 'general',
      priority: 2,
      subject: '',
      content: '',
      refund_amount: '',
      payee_type: 'alipay',
      payee_name: '',
      payee_account: '',
      payee_bank: '',
      contact: '',
      reason: '',
    });
  }, [visible]);

  useEffect(() => {
    if (ticketType === 'refund' && visible && step === 'details') {
      loadUserQuota();
      API.get('/api/ticket/refund/invoice-check').then((res) => {
        if (res.data?.success) setInvoiceConflict(res.data.data);
      });
    } else {
      setInvoiceConflict(null);
      setInvoiceConflictAcked(false);
    }
  }, [ticketType, visible, step]);

  const handleSubmit = async (values) => {
    setLoading(true);
    try {
      if (ticketType === 'refund') {
        const refundAmountNum = Number(values.refund_amount);
        if (!Number.isFinite(refundAmountNum) || refundAmountNum <= 0) {
          showError(t('申请退款金额必须大于 0'));
          setLoading(false);
          return;
        }
        const refundQuota = displayAmountToQuota(refundAmountNum);
        if (!Number.isFinite(refundQuota) || refundQuota <= 0) {
          showError(t('申请退款金额必须大于 0'));
          setLoading(false);
          return;
        }
        if (requestableQuota <= 0) {
          showError(t('无法获取最大可退额度，请刷新后重试'));
          setLoading(false);
          return;
        }
        if (refundQuota > requestableQuota) {
          showError(t('申请退款金额不能超过最大可退额度'));
          setLoading(false);
          return;
        }
        const payload = {
          finance_quote: financeQuote?.quote_token,
          subject: values.subject,
          priority: values.priority,
          refund_quota: refundQuota,
          payee_type: values.payee_type,
          payee_name: values.payee_name,
          payee_account: values.payee_account,
          payee_bank: values.payee_bank,
          contact: values.contact,
          reason: values.reason || values.content,
          invoice_conflict_acknowledged: invoiceConflictAcked,
        };
        const signature = JSON.stringify(payload);
        if (refundRequest.current.signature !== signature)
          refundRequest.current = { signature, key: crypto.randomUUID() };
        const res = await API.post('/api/ticket/refund/', {
          ...payload,
          request_key: refundRequest.current.key,
        });
        if (res.data?.success) {
          showSuccess(
            t(
              'Refund request registered; awaiting settlement verification. No balance has been frozen.',
            ),
          );
          // 通知全局：用户额度已变化，已订阅的组件（如个人中心）可据此刷新。
          try {
            window.dispatchEvent(new CustomEvent('user-quota-changed'));
          } catch (e) {
            // 某些浏览器环境可能不支持 CustomEvent 构造，忽略即可
          }
          onSuccess?.(res.data?.data);
          onClose?.();
        } else {
          if (res.data?.message === 'invoice tax quote changed')
            setQuoteRevision((value) => value + 1);
          showError(t(res.data?.message || '工单创建失败'));
        }
        return;
      }

      // 发票工单有独立端点不走这里；这里只处理 general / 其它可带附件的普通工单。
      const res = await API.post('/api/ticket/', {
        subject: values.subject,
        type: ticketType,
        priority: values.priority,
        content: values.content,
        attachment_ids: attachments.map((a) => a.id),
      });
      if (res.data?.success) {
        showSuccess(t('工单创建成功'));
        reset();
        onSuccess?.(res.data?.data);
        onClose?.();
      } else {
        showError(res.data?.message || t('工单创建失败'));
      }
    } catch (error) {
      showError(t('请求失败'));
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = async () => {
    // 主动撤回已上传但未提交的附件，立即释放存储空间。
    await discardAll();
    onClose?.();
  };

  if (step === 'invoice') {
    return (
      <CreateInvoiceTicketModal
        visible={visible}
        onClose={onClose}
        onSuccess={onSuccess}
        t={t}
        onBack={() => setStep('choose')}
      />
    );
  }

  return (
    <Modal
      title={
        step === 'choose'
          ? t('Choose ticket type')
          : t(
              ticketType === 'refund'
                ? 'ticket.choice.refund'
                : 'ticket.choice.general',
            )
      }
      visible={visible}
      onCancel={handleCancel}
      onOk={() => formApiRef.current?.submitForm()}
      okText={t('提交工单')}
      cancelText={t('取消')}
      confirmLoading={loading || uploading}
      footer={
        step === 'choose' ? null : (
          <div className='flex justify-between gap-3'>
            <Button
              icon={<ArrowLeft size={16} />}
              disabled={loading || uploading}
              onClick={() => setStep('choose')}
            >
              {t('Back')}
            </Button>
            <Button
              theme='solid'
              loading={loading || uploading}
              disabled={
                quoteLoading ||
                !!quoteError ||
                limitStatus?.limited ||
                (ticketType === 'refund' &&
                  (quotaLoading ||
                    eligibilityError ||
                    !eligibility ||
                    (invoiceConflict?.has_active_invoices &&
                      !invoiceConflictAcked)))
              }
              onClick={() => formApiRef.current?.submitForm()}
            >
              {t('提交工单')}
            </Button>
          </div>
        )
      }
      okButtonProps={{
        disabled:
          quoteLoading ||
          !!quoteError ||
          limitStatus?.limited ||
          (invoiceConflict?.has_active_invoices && !invoiceConflictAcked),
      }}
      centered
      width={560}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
    >
      {step === 'choose' && (
        <div className='grid gap-3'>
          {[
            {
              value: 'general',
              label: 'ticket.choice.general',
              Icon: MessageSquare,
            },
            { value: 'invoice', label: 'ticket.choice.invoice', Icon: Receipt },
            { value: 'refund', label: 'ticket.choice.refund', Icon: Wallet },
          ].map(({ value, label, Icon }) => (
            <button
              key={value}
              type='button'
              className='flex min-h-20 w-full items-center gap-4 rounded-lg border p-4 text-left hover:border-blue-500 hover:bg-blue-50 dark:hover:bg-gray-800 focus-visible:outline-2 focus-visible:outline-blue-500'
              onClick={() => {
                if (value === 'invoice') {
                  setStep('invoice');
                  return;
                }
                if (value !== ticketType) {
                  formApiRef.current?.reset();
                  void discardAll();
                }
                formApiRef.current?.setValue('type', value);
                setTicketType(value);
                setStep('details');
              }}
            >
              <Icon size={24} className='shrink-0' aria-hidden='true' />
              <span className='flex-1 font-medium'>{t(label)}</span>
              <ChevronRight size={16} className='shrink-0' aria-hidden='true' />
            </button>
          ))}
        </div>
      )}
      <div
        hidden={step === 'choose'}
        onPasteCapture={(e) => {
          // 包一层 div 挂 paste 监听：Modal 内部 content 渲染层不一定透传 React 合成事件。
          // 退款工单分支不展示 Upload，handlePaste 内部通过 uploadRef 判空自行忽略。
          if (!config.enabled) return;
          handlePaste(e);
        }}
      >
        {limitStatus?.limited && (
          <Banner
            type='warning'
            closeIcon={null}
            description={
              <Text>
                {t(
                  "You've used this week's limit for creating tickets on a low balance. If you need help, please contact support first.",
                )}
              </Text>
            }
            className='!mb-3'
          />
        )}
        <Form
          layout='vertical'
          initValues={{
            type: 'general',
            priority: 2,
            subject: '',
            content: '',
            refund_quota: '',
            refund_amount: '',
            payee_type: 'alipay',
            payee_name: '',
            payee_account: '',
            payee_bank: '',
            contact: '',
            reason: '',
          }}
          getFormApi={(api) => {
            formApiRef.current = api;
          }}
          onValueChange={(values) => {
            setRefundAmount(values?.refund_amount ?? '');
            if (values?.type && values.type !== ticketType) {
              setTicketType(values.type);
            }
            if (values?.payee_type && values.payee_type !== payeeType) {
              setPayeeType(values.payee_type);
            }
          }}
          onSubmit={handleSubmit}
        >
          {ticketType === 'refund' && (
            <>
              {financeQuote && <RefundFinanceDetail finance={financeQuote} />}
              {quoteError && (
                <p role='alert' className='text-red-600'>
                  {t(quoteError)}
                </p>
              )}
              <Banner
                type='info'
                closeIcon={null}
                description={
                  quotaLoading
                    ? t('加载中...')
                    : `${t('当前可用额度')}：${renderQuotaWithAmount(
                        Number(quotaToDisplayAmount(userQuota).toFixed(6)),
                      )}`
                }
                className='!mb-3'
              />
              <Banner
                type='info'
                closeIcon={null}
                description={
                  quotaLoading
                    ? t('加载中...')
                    : `${t('Verified refundable balance')}：${renderQuotaWithAmount(
                        Number(
                          quotaToDisplayAmount(maxRefundableQuota).toFixed(6),
                        ),
                      )}`
                }
                className='!mb-3'
              />
              <Banner
                type='warning'
                closeIcon={null}
                description={t(
                  'Submitting registers a request awaiting settlement verification. Your balance will not be frozen and API usage can continue. The refundable amount must be confirmed again after verification.',
                )}
                className='!mb-3'
              />
              {eligibility && (
                <div className='mb-3 space-y-1 text-sm'>
                  <p>
                    {t('Requestable balance')}:{' '}
                    {renderQuotaWithAmount(
                      quotaToDisplayAmount(requestableQuota),
                    )}
                  </p>
                  <p>
                    {t('Requested quota is not the final cash refund amount.')}
                  </p>
                  <p>
                    {t('Non-refundable balance')}:{' '}
                    {renderQuotaWithAmount(
                      quotaToDisplayAmount(eligibility.non_refundable_quota),
                    )}
                  </p>
                  <p>
                    {t('Frozen Amount')}:{' '}
                    {renderQuotaWithAmount(
                      quotaToDisplayAmount(eligibility.frozen_quota),
                    )}
                  </p>
                  {eligibility.reasons.map((reason) => (
                    <p key={reason.code}>
                      {t(`refund_reason.${reason.code}`)}:{' '}
                      {renderQuotaWithAmount(
                        quotaToDisplayAmount(reason.quota),
                      )}
                    </p>
                  ))}
                </div>
              )}
              {eligibilityError && (
                <p role='alert'>
                  {t('Unable to verify refundable quota, please try again')}
                </p>
              )}
              {invoiceConflict?.has_active_invoices && (
                <Banner
                  type='warning'
                  description={t(
                    '您存在发票申请，提交退款后，关联的发票将会进行冲红处理',
                  )}
                  closeIcon={null}
                  className='!mb-3'
                />
              )}
              {invoiceConflict?.has_active_invoices && (
                <Checkbox
                  checked={invoiceConflictAcked}
                  onChange={(e) => setInvoiceConflictAcked(e.target.checked)}
                  style={{ marginBottom: 12 }}
                >
                  {t('我已了解退款可能导致发票红冲，确认继续提交')}
                </Checkbox>
              )}
            </>
          )}

          <Form.Input
            field='subject'
            label={t('工单主题')}
            maxLength={255}
            showClear
            placeholder={
              ticketType === 'refund'
                ? t('可留空，将自动生成"退款申请"主题')
                : t('请简要描述问题')
            }
            rules={
              ticketType === 'refund'
                ? []
                : [{ required: true, message: t('工单主题不能为空') }]
            }
          />
          <Form.Select
            field='priority'
            label={t('优先级')}
            optionList={getTicketPriorityOptions(t)}
            getPopupContainer={() => document.body}
          />

          {ticketType === 'refund' ? (
            <>
              <Form.InputNumber
                field='refund_amount'
                label={t('申请退款金额')}
                prefix={getCurrencyConfig().symbol}
                placeholder={t('请输入希望退还的金额')}
                precision={6}
                step={0.000001}
                min={0}
                max={Number(quotaToDisplayAmount(requestableQuota).toFixed(6))}
                style={{ width: '100%' }}
                rules={[
                  { required: true, message: t('申请退款金额必须大于 0') },
                  {
                    validator: (_, value) => {
                      const num = Number(value);
                      if (!Number.isFinite(num) || num <= 0) return false;
                      if (displayAmountToQuota(num) > requestableQuota)
                        return false;
                      return true;
                    },
                    message: t('申请退款金额不能超过最大可退额度'),
                  },
                ]}
              />
              <Form.Select
                field='payee_type'
                label={t('收款方式')}
                optionList={getRefundPayeeTypeOptions(t)}
                getPopupContainer={() => document.body}
                rules={[{ required: true, message: t('请选择收款方式') }]}
              />
              <Form.Input
                field='payee_name'
                label={t('收款人姓名')}
                maxLength={128}
                showClear
                placeholder={t('请输入收款人真实姓名')}
                rules={[{ required: true, message: t('收款人姓名不能为空') }]}
              />
              <Form.Input
                field='payee_account'
                label={t('收款账号')}
                maxLength={128}
                showClear
                placeholder={t('支付宝账号或银行卡号')}
                rules={[{ required: true, message: t('收款账号不能为空') }]}
              />
              {payeeType === 'bank' && (
                <Form.Input
                  field='payee_bank'
                  label={t('开户行')}
                  maxLength={255}
                  showClear
                  placeholder={t('请输入开户行名称')}
                  rules={[{ required: true, message: t('开户行不能为空') }]}
                />
              )}
              <Form.Input
                field='contact'
                label={t('联系方式')}
                maxLength={128}
                showClear
                placeholder={t('手机号或邮箱，便于管理员联系')}
                rules={[{ required: true, message: t('联系方式不能为空') }]}
              />
              <Form.TextArea
                field='reason'
                label={t('退款原因')}
                autosize={{ minRows: 3, maxRows: 6 }}
                maxLength={5000}
                showClear
                placeholder={t('请简要说明退款原因')}
                rules={[{ required: true, message: t('工单内容不能为空') }]}
              />
            </>
          ) : (
            <>
              <Form.TextArea
                field='content'
                label={t('问题描述')}
                autosize={{ minRows: 4, maxRows: 8 }}
                maxLength={5000}
                showClear
                placeholder={t('请详细描述问题，方便管理员更快定位')}
                rules={[{ required: true, message: t('工单内容不能为空') }]}
              />
              {config.enabled && (
                <Form.Slot label={t('附件（可选）')}>
                  <Upload {...uploadProps} ref={uploadRef} disabled={loading}>
                    <Button icon={<IconUpload />} disabled={loading}>
                      {t('上传附件')}
                    </Button>
                    <Text type='tertiary' style={{ marginLeft: 8 }}>
                      {t('最多 {{n}} 个，单个不超过 {{mb}} MB，可粘贴截图', {
                        n: config.maxCount,
                        mb: Math.floor(config.maxSize / 1024 / 1024),
                      })}
                    </Text>
                  </Upload>
                </Form.Slot>
              )}
            </>
          )}
        </Form>
      </div>
    </Modal>
  );
};

export default CreateTicketModal;

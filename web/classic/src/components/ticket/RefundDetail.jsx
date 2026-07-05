import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Descriptions,
  Empty,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Space,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import {
  API,
  copy,
  getCurrencyConfig,
  renderQuotaWithAmount,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../helpers/quota';
import {
  getRefundPayeeTypeText,
  getRefundStatusColor,
  getRefundStatusText,
} from './ticketUtils';

const { Title, Text } = Typography;

const REFUND_STATUS_PENDING = 1;
const REFUND_STATUS_REFUNDED = 2;
const REFUND_STATUS_REJECTED = 3;

const QUOTA_MODE_WRITE_OFF = 'write_off';
const QUOTA_MODE_SUBTRACT = 'subtract';
const QUOTA_MODE_OVERRIDE = 'override';

const formatAmount = (quota) =>
  renderQuotaWithAmount(Number(quotaToDisplayAmount(quota || 0).toFixed(6)));

const CopyableText = ({ value, t }) => {
  if (!value) return <Text>-</Text>;
  return (
    <Space spacing={4} align='center'>
      <Text>{value}</Text>
      <Button
        theme='borderless'
        size='small'
        icon={<IconCopy />}
        onClick={async () => {
          if (await copy(value)) showSuccess(t('已复制'));
        }}
      />
    </Space>
  );
};

const RefundDetail = ({
  refund,
  ticket,
  loading = false,
  readonly = false,
  onStatusChange,
  onSendMessage,
  t,
}) => {
  const [resolveVisible, setResolveVisible] = useState(false);
  const [resolveLoading, setResolveLoading] = useState(false);
  const [rejectVisible, setRejectVisible] = useState(false);
  const [rejectReason, setRejectReason] = useState('');
  const [rejectLoading, setRejectLoading] = useState(false);

  const [quotaMode, setQuotaMode] = useState(QUOTA_MODE_WRITE_OFF);
  const [customAmountInput, setCustomAmountInput] = useState('');
  const [targetUserQuota, setTargetUserQuota] = useState(null);
  const [targetUserQuotaLoading, setTargetUserQuotaLoading] = useState(false);

  const currencySymbol = getCurrencyConfig().symbol;
  const targetUserId = ticket?.user_id || refund?.user_id || 0;

  useEffect(() => {
    if (!resolveVisible) return;
    setQuotaMode(QUOTA_MODE_WRITE_OFF);
    setCustomAmountInput('');
    if (!targetUserId) {
      setTargetUserQuota(null);
      return;
    }
    let cancelled = false;
    (async () => {
      setTargetUserQuotaLoading(true);
      try {
        const res = await API.get(`/api/user/${targetUserId}`);
        if (!cancelled && res.data?.success) {
          setTargetUserQuota(Number(res.data?.data?.quota || 0));
        }
      } catch (error) {
        // 仅用于预览，读取失败不阻塞操作
      } finally {
        if (!cancelled) setTargetUserQuotaLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [resolveVisible, targetUserId]);

  const payeeRows = useMemo(() => {
    if (!refund) return [];
    return [
      {
        key: t('收款方式'),
        value: (
          <Tag color='blue' shape='circle'>
            {getRefundPayeeTypeText(refund.payee_type, t)}
          </Tag>
        ),
      },
      {
        key: t('收款人姓名'),
        value: <CopyableText value={refund.payee_name} t={t} />,
      },
      {
        key: t('收款账号'),
        value: <CopyableText value={refund.payee_account} t={t} />,
      },
      ...(refund.payee_bank
        ? [
            {
              key: t('开户行'),
              value: <CopyableText value={refund.payee_bank} t={t} />,
            },
          ]
        : []),
      {
        key: t('联系方式'),
        value: <CopyableText value={refund.contact} t={t} />,
      },
    ];
  }, [refund, t]);

  const customQuotaFromInput = () => {
    const str = String(customAmountInput ?? '').trim();
    if (str === '') return null;
    const num = Number(str);
    if (!Number.isFinite(num) || num < 0) return null;
    return displayAmountToQuota(num);
  };

  const parsedCustomQuota = customQuotaFromInput();

  const isResolveConfirmEnabled = (() => {
    if (!refund) return false;
    if (quotaMode === QUOTA_MODE_WRITE_OFF) return true;
    if (parsedCustomQuota === null) return false;
    if (quotaMode === QUOTA_MODE_SUBTRACT) {
      if (parsedCustomQuota <= 0) return false;
      // 扣减金额不能超过 解冻后可扣额度；若已知用户当前 quota，则上限 = targetUserQuota + frozen。
      if (targetUserQuota !== null) {
        const available = targetUserQuota + (refund.frozen_quota || 0);
        if (parsedCustomQuota > available) return false;
      }
      return true;
    }
    // override：≥0 即可
    return parsedCustomQuota >= 0;
  })();

  const resolvePreviewText = (() => {
    if (!refund) return '';
    const frozenQ = refund.frozen_quota || 0;
    if (quotaMode === QUOTA_MODE_SUBTRACT) {
      if (parsedCustomQuota === null || parsedCustomQuota <= 0) {
        return t('请输入大于 0 的扣减金额');
      }
      if (targetUserQuota === null) {
        return `${t('将从解冻后余额中扣减')} ${formatAmount(parsedCustomQuota)}`;
      }
      const after = targetUserQuota + frozenQ - parsedCustomQuota;
      if (after < 0) {
        return t('扣减金额超过用户解冻后的可用余额');
      }
      return `${t('解冻后余额')} ${formatAmount(targetUserQuota + frozenQ)} − ${formatAmount(parsedCustomQuota)} = ${formatAmount(after)}`;
    }
    if (quotaMode === QUOTA_MODE_OVERRIDE) {
      if (parsedCustomQuota === null) {
        return t('请输入用户最终余额（可为 0）');
      }
      if (targetUserQuota === null) {
        return `${t('最终余额将被设置为')} ${formatAmount(parsedCustomQuota)}`;
      }
      return `${t('解冻后余额')} ${formatAmount(targetUserQuota + frozenQ)} → ${formatAmount(parsedCustomQuota)}`;
    }
    return '';
  })();

  const handleResolveConfirm = async () => {
    let extra;
    if (quotaMode === QUOTA_MODE_SUBTRACT) {
      const quota = customQuotaFromInput();
      if (!quota || quota <= 0) return;
      extra = { quota_mode: QUOTA_MODE_SUBTRACT, actual_refund_quota: quota };
    } else if (quotaMode === QUOTA_MODE_OVERRIDE) {
      const quota = customQuotaFromInput();
      if (quota === null) return;
      extra = { quota_mode: QUOTA_MODE_OVERRIDE, actual_refund_quota: quota };
    } else {
      extra = { quota_mode: QUOTA_MODE_WRITE_OFF };
    }
    setResolveLoading(true);
    try {
      await onStatusChange?.(REFUND_STATUS_REFUNDED, extra);
      setResolveVisible(false);
    } finally {
      setResolveLoading(false);
    }
  };

  const handleRejectConfirm = async () => {
    setRejectLoading(true);
    try {
      const reason = String(rejectReason || '').trim();
      if (reason && typeof onSendMessage === 'function') {
        await onSendMessage(`${t('驳回理由')}：\n${reason}`);
      }
      await onStatusChange?.(REFUND_STATUS_REJECTED, {});
      setRejectVisible(false);
      setRejectReason('');
    } finally {
      setRejectLoading(false);
    }
  };

  if (!refund) {
    return (
      <Card className='!rounded-2xl shadow-sm border-0'>
        <Title heading={5} className='!mb-1'>
          {t('退款信息')}
        </Title>
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('暂无退款信息')}
        />
      </Card>
    );
  }

  const isPending = refund.refund_status === REFUND_STATUS_PENDING;
  const frozenAmount = formatAmount(refund.frozen_quota || refund.refund_quota);
  const requestedAmount = formatAmount(refund.refund_quota);
  const snapshotAmount = formatAmount(refund.user_quota_snapshot);

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex flex-col gap-4'>
        {/* 头部：标题 + 状态 + 操作按钮 */}
        <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
          <Space align='center'>
            <Title heading={5} className='!mb-0'>
              {t('退款信息')}
            </Title>
            <Tag
              color={getRefundStatusColor(refund.refund_status)}
              shape='circle'
            >
              {getRefundStatusText(refund.refund_status, t)}
            </Tag>
          </Space>
          {!readonly && typeof onStatusChange === 'function' && (
            <Space wrap>
              <Button
                theme='light'
                type='danger'
                loading={loading}
                disabled={!isPending}
                onClick={() => {
                  setRejectReason('');
                  setRejectVisible(true);
                }}
              >
                {t('驳回并解冻')}
              </Button>
              <Button
                theme='solid'
                type='primary'
                loading={loading}
                disabled={!isPending}
                onClick={() => setResolveVisible(true)}
              >
                {t('完成退款')}
              </Button>
            </Space>
          )}
        </div>

        {/* 金额高亮区 */}
        <div className='grid grid-cols-1 md:grid-cols-3 gap-3'>
          <div
            className='rounded-2xl p-4'
            style={{ background: 'var(--semi-color-primary-light-default)' }}
          >
            <Text type='secondary' size='small'>
              {t('申请退款金额')}
            </Text>
            <div className='mt-1'>
              <Text
                strong
                style={{
                  fontSize: 24,
                  color: 'var(--semi-color-primary)',
                }}
              >
                {requestedAmount}
              </Text>
            </div>
          </div>
          <div
            className='rounded-2xl p-4'
            style={{
              background: isPending
                ? 'var(--semi-color-warning-light-default)'
                : 'var(--semi-color-fill-0)',
            }}
          >
            <Text type='secondary' size='small'>
              {isPending ? t('冻结中金额') : t('冻结记录')}
            </Text>
            <div className='mt-1'>
              <Text
                strong
                style={{
                  fontSize: 20,
                  color: isPending ? 'var(--semi-color-warning)' : undefined,
                }}
              >
                {frozenAmount}
              </Text>
            </div>
            <Text type='tertiary' size='small'>
              {isPending
                ? t('该金额已从用户可用额度中预扣除')
                : t('工单处理完成后冻结已解除')}
            </Text>
          </div>
          <div
            className='rounded-2xl p-4'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <Text type='secondary' size='small'>
              {t('提交时可用额度')}
            </Text>
            <div className='mt-1'>
              <Text strong style={{ fontSize: 20 }}>
                {snapshotAmount}
              </Text>
            </div>
          </div>
        </div>

        {/* 收款信息 */}
        <div>
          <Text strong className='block mb-2'>
            {t('收款信息')}
          </Text>
          <Descriptions data={payeeRows} />
        </div>

        {/* 退款原因 */}
        <div>
          <Text strong className='block mb-2'>
            {t('退款原因')}
          </Text>
          <div
            className='rounded-xl p-3'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            {refund.reason ? (
              <Text style={{ whiteSpace: 'pre-wrap' }}>{refund.reason}</Text>
            ) : (
              <Text type='tertiary'>{t('用户未填写')}</Text>
            )}
          </div>
        </div>

        {/* 时间 */}
        <div className='flex flex-wrap gap-x-6 gap-y-1'>
          <Text type='tertiary' size='small'>
            {t('提交时间')}：
            {refund.created_time ? timestamp2string(refund.created_time) : '-'}
          </Text>
          <Text type='tertiary' size='small'>
            {t('处理时间')}：
            {refund.processed_time
              ? timestamp2string(refund.processed_time)
              : '-'}
          </Text>
        </div>
      </div>

      {/* 驳回 Modal */}
      <Modal
        centered
        visible={rejectVisible}
        onOk={handleRejectConfirm}
        onCancel={() => {
          setRejectVisible(false);
          setRejectReason('');
        }}
        confirmLoading={rejectLoading}
        okText={t('确认驳回')}
        okButtonProps={{ type: 'danger', theme: 'solid' }}
        cancelText={t('取消')}
        title={t('驳回退款申请')}
      >
        <div className='mb-3'>
          <Text type='secondary'>
            {t(
              '驳回后工单回到处理中状态，冻结的额度会立即归还给用户可用额度，用户可再次发起退款申请。',
            )}
          </Text>
        </div>
        <div>
          <div className='mb-1'>
            <Text size='small'>
              {t('驳回理由')}
              <Text type='tertiary' size='small' className='ml-1'>
                （{t('选填，会作为一条回复发送给用户')}）
              </Text>
            </Text>
          </div>
          <TextArea
            value={rejectReason}
            onChange={setRejectReason}
            autosize={{ minRows: 3, maxRows: 6 }}
            maxLength={2000}
            placeholder={t('例如：收款信息有误，请补充更新后再提交')}
            showClear
          />
        </div>
      </Modal>

      {/* 完成退款 Modal */}
      <Modal
        centered
        visible={resolveVisible}
        onOk={handleResolveConfirm}
        onCancel={() => setResolveVisible(false)}
        confirmLoading={resolveLoading}
        okText={t('确认完成退款')}
        okButtonProps={{ disabled: !isResolveConfirmEnabled }}
        cancelText={t('取消')}
        title={t('完成退款')}
        bodyStyle={{
          maxHeight: 'calc(80vh - 120px)',
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        <div className='flex flex-col gap-3'>
          <Banner
            type='info'
            closeIcon={null}
            description={t(
              '请根据实际线下打款情况选择额度处理方式。操作将写入用户日志，便于日后追溯。',
            )}
          />

          <div
            className='rounded-xl p-3'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <div className='flex flex-wrap gap-x-6 gap-y-1'>
              <Text type='secondary' size='small'>
                {t('申请退款金额')}：
                <Text strong>{requestedAmount}</Text>
              </Text>
              <Text type='secondary' size='small'>
                {t('当前冻结金额')}：
                <Text strong>{frozenAmount}</Text>
              </Text>
              <Text type='secondary' size='small'>
                {t('用户当前余额')}：
                <Text strong>
                  {targetUserQuotaLoading || targetUserQuota === null
                    ? '-'
                    : formatAmount(targetUserQuota)}
                </Text>
              </Text>
            </div>
            <div className='mt-2'>
              <Text type='tertiary' size='small'>
                {t('收款方')}：{refund.payee_name} ·{' '}
                {getRefundPayeeTypeText(refund.payee_type, t)} ·{' '}
                {refund.payee_account}
              </Text>
            </div>
          </div>

          <div>
            <div className='mb-2'>
              <Text size='small' strong>
                {t('额度处理方式')}
              </Text>
            </div>
            <RadioGroup
              value={quotaMode}
              onChange={(e) => setQuotaMode(e.target.value)}
              direction='vertical'
            >
              <Radio value={QUOTA_MODE_WRITE_OFF}>
                <Text>{t('核销冻结额度')}</Text>
                <Text type='tertiary' size='small' className='ml-2'>
                  {t('推荐：按冻结金额')}{frozenAmount}
                  {t('核销，用户余额不变')}
                </Text>
              </Radio>
              <Radio value={QUOTA_MODE_SUBTRACT}>
                <Text>{t('自定义扣减额度')}</Text>
                <Text type='tertiary' size='small' className='ml-2'>
                  {t('先解冻回用户余额，再按下方金额扣减')}
                </Text>
              </Radio>
              <Radio value={QUOTA_MODE_OVERRIDE}>
                <Text>{t('覆盖最终余额')}</Text>
                <Text type='tertiary' size='small' className='ml-2'>
                  {t('先解冻回用户余额，再把余额设为下方金额')}
                </Text>
              </Radio>
            </RadioGroup>
          </div>

          {quotaMode !== QUOTA_MODE_WRITE_OFF && (
            <div>
              <div className='mb-1'>
                <Text size='small'>
                  {quotaMode === QUOTA_MODE_SUBTRACT
                    ? t('实际扣减金额')
                    : t('用户最终余额')}
                </Text>
              </div>
              <InputNumber
                value={customAmountInput}
                onChange={(v) => setCustomAmountInput(v)}
                prefix={currencySymbol}
                placeholder={
                  quotaMode === QUOTA_MODE_SUBTRACT
                    ? t('请输入实际扣减金额')
                    : t('请输入用户最终余额')
                }
                precision={6}
                step={0.000001}
                min={0}
                style={{ width: '100%' }}
              />
              <div className='mt-1'>
                <Text type='tertiary' size='small'>
                  {resolvePreviewText}
                </Text>
              </div>
            </div>
          )}
        </div>
      </Modal>
    </Card>
  );
};

export default RefundDetail;

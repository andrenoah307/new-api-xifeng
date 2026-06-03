import React, { useState, useEffect } from 'react';
import { Modal, Input, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

export default function OfflineExportModal({
  visible,
  onClose,
  onSubmit,
  submitting,
  filters,
}) {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');

  useEffect(() => {
    if (visible) {
      setEmail('');
    }
  }, [visible]);

  const handleOk = () => {
    if (!email.trim()) {
      return;
    }
    const exportFilters = {};
    if (filters?.start_timestamp) {
      exportFilters.start_timestamp = filters.start_timestamp;
    }
    if (filters?.end_timestamp) {
      exportFilters.end_timestamp = filters.end_timestamp;
    }
    if (filters?.model_name) {
      exportFilters.model_name = filters.model_name;
    }
    if (filters?.token_name) {
      exportFilters.token_name = filters.token_name;
    }
    if (filters?.channel) {
      exportFilters.channel_id = Number(filters.channel);
    }
    onSubmit(exportFilters, email.trim());
  };

  const formatTime = (val) => {
    if (!val) return '-';
    // val is epoch seconds from buildOfflineFilters
    const d = new Date(val * 1000);
    if (isNaN(d.getTime())) return String(val);
    return d.toLocaleString();
  };

  return (
    <Modal
      title={t('离线导出')}
      visible={visible}
      onOk={handleOk}
      onCancel={onClose}
      okText={t('提交导出任务')}
      cancelText={t('取消')}
      confirmLoading={submitting}
      centered
      bodyStyle={{ maxHeight: 'calc(80vh - 120px)', overflowY: 'auto', overflowX: 'hidden' }}
    >
      <div style={{ marginBottom: 16 }}>
        <Text type="secondary">{t('当前筛选条件')}</Text>
        <div style={{ background: 'var(--semi-color-fill-0)', borderRadius: 8, padding: 12, marginTop: 8, fontSize: 13 }}>
          <div>{t('时间范围')}: {formatTime(filters?.start_timestamp)} ~ {formatTime(filters?.end_timestamp)}</div>
          {filters?.model_name && <div>{t('模型名称')}: {filters.model_name}</div>}
          {filters?.token_name && <div>{t('令牌名称')}: {filters.token_name}</div>}
          {filters?.channel && <div>{t('渠道 ID')}: {filters.channel}</div>}
        </div>
      </div>
      <div style={{ marginBottom: 12 }}>
        <Text size="small" style={{ display: 'block', marginBottom: 4 }}>{t('通知邮箱')}</Text>
        <Input
          placeholder={t('导出完成后将发送邮件通知')}
          value={email}
          onChange={setEmail}
          size="default"
        />
      </div>
      <Text type="tertiary" size="small">
        {t('导出将在后台异步执行，完成后通过邮件发送下载链接')}
      </Text>
    </Modal>
  );
}

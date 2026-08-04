import React from 'react';
import { Modal, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { showError } from '../../../../helpers';

const { Text } = Typography;

export default function OfflineExportModal({
  visible,
  onClose,
  onSubmit,
  submitting,
  filters,
  userEmail,
}) {
  const { t } = useTranslation();

  const handleOk = () => {
    if (!filters?.start_timestamp || !filters?.end_timestamp) {
      showError(t('请选择日志记录时间'));
      return;
    }

    const exportFilters = {
      start_timestamp: filters.start_timestamp,
      end_timestamp: filters.end_timestamp,
    };
    if (filters?.model_name) {
      exportFilters.model_name = filters.model_name;
    }
    if (filters?.token_name) {
      exportFilters.token_name = filters.token_name;
    }
    onSubmit(exportFilters);
  };

  const formatTime = (val) => {
    if (!val) return '-';
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
        </div>
      </div>
      <div style={{ marginBottom: 12 }}>
        <Text size="small" style={{ display: 'block', marginBottom: 4 }}>
          {t('通知邮箱')}: <Text strong>{userEmail || t('未绑定邮箱，请在账号设置中绑定')}</Text>
        </Text>
      </div>
      <Text type="tertiary" size="small">
        {t('离线导出限制：每 24 小时可提交一次，结果保留 72 小时')}
      </Text>
    </Modal>
  );
}

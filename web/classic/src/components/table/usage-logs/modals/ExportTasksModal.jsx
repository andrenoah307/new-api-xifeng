import React, { useEffect } from 'react';
import { Modal, Table, Tag, Button, Progress, Typography } from '@douyinfe/semi-ui';
import { IconDownload } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { timestamp2string, getUserIdFromLocalStorage } from '../../../../helpers';
import { showError } from '../../../../helpers';

const { Text } = Typography;

const STATUS_MAP = {
  0: { text: '待处理', color: 'amber' },
  1: { text: '处理中', color: 'blue' },
  2: { text: '已完成', color: 'green' },
  3: { text: '失败', color: 'red' },
  4: { text: '已取消', color: 'grey' },
};

function formatFileSize(bytes) {
  if (!bytes || bytes <= 0) return '-';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

export default function ExportTasksModal({
  visible,
  onClose,
  tasks,
  total,
  page,
  loading,
  onPageChange,
  onRefresh,
}) {
  const { t } = useTranslation();

  useEffect(() => {
    if (visible) {
      onRefresh?.(1);
    }
  }, [visible]);

  useEffect(() => {
    if (!visible) return;
    const interval = setInterval(() => onRefresh?.(page), 10000);
    return () => clearInterval(interval);
  }, [visible, page, onRefresh]);

  const columns = [
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 90,
      render: (status) => {
        const cfg = STATUS_MAP[status] || STATUS_MAP[4];
        return <Tag color={cfg.color} size="small">{t(cfg.text)}</Tag>;
      },
    },
    {
      title: t('进度'),
      dataIndex: 'progress',
      width: 80,
      render: (progress, record) => {
        if (record.status === 1) {
          return <Progress percent={progress} size="small" style={{ width: 60 }} />;
        }
        return '-';
      },
    },
    {
      title: t('行数'),
      dataIndex: 'row_count',
      width: 80,
      render: (val) => (val > 0 ? val.toLocaleString() : '-'),
    },
    {
      title: t('文件大小'),
      dataIndex: 'file_size',
      width: 90,
      render: (val) => formatFileSize(val),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      width: 150,
      render: (val) => (val ? timestamp2string(val) : '-'),
    },
    {
      title: t('操作'),
      width: 80,
      render: (_, record) => {
        if (record.status === 2) {
          return (
            <Button
              icon={<IconDownload />}
              size="small"
              type="tertiary"
              onClick={async () => {
                try {
                  const res = await fetch(`/api/log/self/export-download/${record.id}`, {
                    credentials: 'include',
                    headers: { 'New-API-User': String(getUserIdFromLocalStorage()) },
                  });
                  if (!res.ok) throw new Error(res.statusText);
                  const blob = await res.blob();
                  const url = URL.createObjectURL(blob);
                  const a = document.createElement('a');
                  a.href = url;
                  a.download = `${record.username || 'export'}-${timestamp2string(record.created_time).replaceAll('-', '').replaceAll(':', '').replace(' ', '-')}.csv.gz`;
                  document.body.appendChild(a);
                  a.click();
                  a.remove();
                  URL.revokeObjectURL(url);
                } catch (e) {
                  showError(e.message || t('下载失败'));
                }
              }}
            >
              {t('下载')}
            </Button>
          );
        }
        if (record.status === 3 && record.error_message) {
          return <Text type="danger" size="small">{record.error_message}</Text>;
        }
        return null;
      },
    },
  ];

  return (
    <Modal
      title={t('导出任务记录')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={700}
      centered
      bodyStyle={{ maxHeight: 'calc(80vh - 120px)', overflowY: 'auto', overflowX: 'hidden' }}
      style={{ maxWidth: '92vw' }}
    >
      <Table
        columns={columns}
        dataSource={tasks}
        rowKey="id"
        loading={loading}
        pagination={{
          currentPage: page,
          pageSize: 10,
          total,
          onPageChange: (p) => onPageChange?.(p),
        }}
        size="small"
        empty={t('暂无导出任务')}
      />
    </Modal>
  );
}

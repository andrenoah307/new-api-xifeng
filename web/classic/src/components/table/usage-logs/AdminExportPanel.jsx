import React, { useState, useEffect, useCallback } from 'react';
import { Table, Card, Tag, Button, Typography, Modal } from '@douyinfe/semi-ui';
import { IconDownload, IconClose, IconRefresh } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, timestamp2string, getUserIdFromLocalStorage } from '../../../helpers';

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

export default function AdminExportPanel() {
  const { t } = useTranslation();
  const [tasks, setTasks] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [stats, setStats] = useState(null);

  const loadTasks = useCallback(async (p = page) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/log/export-tasks?page=${p}&page_size=10`);
      if (res.data.success) {
        setTasks(res.data.data?.items || []);
        setTotal(res.data.data?.total || 0);
      }
    } catch {
      showError(t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [page, t]);

  const loadStats = useCallback(async () => {
    try {
      const res = await API.get('/api/log/export-queue-stats');
      if (res.data.success) setStats(res.data.data);
    } catch {
      // silent
    }
  }, []);

  useEffect(() => {
    loadTasks(page);
    loadStats();
  }, [page]);

  useEffect(() => {
    const interval = setInterval(() => {
      loadTasks(page);
      loadStats();
    }, 15000);
    return () => clearInterval(interval);
  }, [page]);

  const cancelTask = (taskId) => {
    Modal.confirm({
      title: t('取消导出任务'),
      content: `${t('任务 ID')}: ${taskId}`,
      centered: true,
      onOk: async () => {
        try {
          const res = await API.post(`/api/log/export-tasks/${taskId}/cancel`);
          if (res.data.success) {
            showSuccess(t('任务已取消'));
            loadTasks(page);
            loadStats();
          } else {
            showError(res.data.message || t('取消失败'));
          }
        } catch {
          showError(t('取消失败'));
        }
      },
    });
  };

  const downloadTask = async (taskId) => {
    try {
      const res = await fetch(`/api/log/export-tasks/${taskId}/download`, {
        credentials: 'include',
        headers: { 'New-API-User': String(getUserIdFromLocalStorage()) },
      });
      if (!res.ok) throw new Error(res.statusText);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `export-${taskId}.csv.gz`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (e) {
      showError(e.message || t('下载失败'));
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: t('用户'), dataIndex: 'username', width: 120, render: (val) => val || '-' },
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
      width: 70,
      render: (val, record) => (record.status === 1 ? `${val}%` : '-'),
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
      width: 120,
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 4 }}>
          {record.status === 2 && (
            <Button icon={<IconDownload />} size="small" type="tertiary" onClick={() => downloadTask(record.id)}>
              {t('下载')}
            </Button>
          )}
          {(record.status === 0 || record.status === 1) && (
            <Button icon={<IconClose />} size="small" type="danger" theme="light" onClick={() => cancelTask(record.id)}>
              {t('取消')}
            </Button>
          )}
        </div>
      ),
    },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {stats && (
        <Card title={t('队列状态')} headerStyle={{ padding: '12px 16px' }} bodyStyle={{ padding: '12px 16px' }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px 24px', fontSize: 13 }}>
            <span><Text type="tertiary">{t('队列深度')}:</Text> {stats.queue_depth} / {stats.queue_capacity}</span>
            <span><Text type="tertiary">{t('工作线程')}:</Text> {stats.worker_count}</span>
            <span><Text type="tertiary">{t('存储')}:</Text> {formatFileSize(stats.total_storage_bytes)}</span>
            <span><Text type="tertiary">{t('待处理')}:</Text> {stats.pending_count}</span>
            <span><Text type="tertiary">{t('处理中')}:</Text> {stats.processing_count}</span>
            <span><Text type="tertiary">{t('已完成')}:</Text> {stats.done_count}</span>
            <span><Text type="tertiary">{t('失败')}:</Text> {stats.failed_count}</span>
          </div>
        </Card>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: -8 }}>
        <Button icon={<IconRefresh />} onClick={() => { loadTasks(page); loadStats(); }} loading={loading}>
          {t('刷新')}
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={tasks}
        rowKey="id"
        loading={loading}
        pagination={{
          currentPage: page,
          pageSize: 10,
          total,
          onPageChange: setPage,
          showSizeChanger: false,
        }}
        size="small"
        empty={t('暂无导出任务')}
      />
    </div>
  );
}

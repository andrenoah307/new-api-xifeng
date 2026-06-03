import { useState, useCallback } from 'react';
import { Toast } from '@douyinfe/semi-ui';
import { API, showError } from '../../helpers';
import { useTranslation } from 'react-i18next';

export function useOfflineExport() {
  const { t } = useTranslation();
  const [modalOpen, setModalOpen] = useState(false);
  const [tasksModalOpen, setTasksModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [tasks, setTasks] = useState([]);
  const [tasksTotal, setTasksTotal] = useState(0);
  const [tasksPage, setTasksPage] = useState(1);
  const [tasksLoading, setTasksLoading] = useState(false);

  const submitOfflineExport = useCallback(async (filters, email) => {
    setSubmitting(true);
    try {
      const res = await API.post('/api/log/self/export-offline', { filters, email });
      const { success, message, next_available_time } = res.data;
      if (success) {
        Toast.success(t('离线导出任务已提交，完成后将通过邮件通知'));
        setModalOpen(false);
        return { success: true };
      }
      showError(message);
      return { success: false, next_available_time };
    } catch (error) {
      showError(error.message || t('提交失败'));
      return { success: false };
    } finally {
      setSubmitting(false);
    }
  }, [t]);

  const fetchTasks = useCallback(async (page = 1) => {
    setTasksLoading(true);
    try {
      const res = await API.get(`/api/log/self/export-tasks?page=${page}&page_size=10`);
      if (res.data.success) {
        setTasks(res.data.data.items || []);
        setTasksTotal(res.data.data.total || 0);
        setTasksPage(page);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setTasksLoading(false);
    }
  }, []);

  return {
    modalOpen, setModalOpen,
    tasksModalOpen, setTasksModalOpen,
    submitting,
    tasks, tasksTotal, tasksPage, tasksLoading,
    submitOfflineExport,
    fetchTasks,
  };
}

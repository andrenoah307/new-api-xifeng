/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useCallback, useEffect, useState } from 'react';
import { Button, Input, Modal, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import CardPro from '../../common/ui/CardPro';
import InvitationCodesTable from './InvitationCodesTable';
import EditInvitationCodeModal from './modals/EditInvitationCodeModal';
import { API, copy, showError, showSuccess } from '../../../helpers';
import { ITEMS_PER_PAGE } from '../../../constants';
import { useTableCompactMode } from '../../../hooks/common/useTableCompactMode';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const { Text } = Typography;

export const INVITATION_CODE_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
};

const InvitationCodesPage = () => {
  const { t } = useTranslation();
  const [invitationCodes, setInvitationCodes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [showEdit, setShowEdit] = useState(false);
  const [editingInvitationCode, setEditingInvitationCode] = useState({
    id: undefined,
    status: INVITATION_CODE_STATUS.ENABLED,
  });
  const [compactMode, setCompactMode] =
    useTableCompactMode('invitation-codes');
  const isMobile = useIsMobile();

  const loadInvitationCodes = useCallback(
    async (page = 1, currentPageSize = pageSize, currentKeyword = keyword) => {
      setLoading(true);
      try {
        const trimmedKeyword = currentKeyword.trim();
        const path =
          trimmedKeyword.length > 0
            ? `/api/invitation_code/search?keyword=${encodeURIComponent(trimmedKeyword)}&p=${page}&page_size=${currentPageSize}`
            : `/api/invitation_code/?p=${page}&page_size=${currentPageSize}`;
        const res = await API.get(path);
        const { success, message, data } = res.data;
        if (success) {
          setInvitationCodes(data.items || []);
          setActivePage(data.page <= 0 ? 1 : data.page);
          setTotal(data.total || 0);
        } else {
          showError(message);
        }
      } catch (error) {
        showError(error.message);
      } finally {
        setLoading(false);
      }
    },
    [keyword, pageSize],
  );

  useEffect(() => {
    loadInvitationCodes(1, pageSize).catch((error) => showError(error));
  }, [loadInvitationCodes, pageSize]);

  const refresh = async (page = activePage) => {
    await loadInvitationCodes(page, pageSize, keyword);
  };

  const handleSearch = async () => {
    setSearching(true);
    try {
      await loadInvitationCodes(1, pageSize, keyword);
    } finally {
      setSearching(false);
    }
  };

  const handlePageChange = (page) => {
    setActivePage(page);
    loadInvitationCodes(page, pageSize, keyword).catch((error) =>
      showError(error),
    );
  };

  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
  };

  const copyText = async (text, successMessage = '已复制到剪贴板！') => {
    if (await copy(text)) {
      showSuccess(t(successMessage));
      return;
    }
    Modal.error({
      title: t('无法复制到剪贴板，请手动复制'),
      content: text,
      size: 'large',
    });
  };

  const openCreateModal = () => {
    setEditingInvitationCode({
      id: undefined,
      status: INVITATION_CODE_STATUS.ENABLED,
    });
    setShowEdit(true);
  };

  const openEditModal = (record) => {
    setEditingInvitationCode(record);
    setShowEdit(true);
  };

  const closeEditModal = () => {
    setShowEdit(false);
    setTimeout(() => {
      setEditingInvitationCode({
        id: undefined,
        status: INVITATION_CODE_STATUS.ENABLED,
      });
    }, 300);
  };

  const updateInvitationCodeStatus = async (record, status) => {
    setLoading(true);
    try {
      const res = await API.put('/api/invitation_code/', {
        id: record.id,
        name: record.name,
        status,
        max_uses: record.max_uses,
        expired_time: record.expired_time,
        owner_user_id: record.owner_user_id,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('操作成功完成！'));
        await refresh();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  const deleteInvitationCode = async (record) => {
    Modal.confirm({
      title: t('确定是否要删除此邀请码？'),
      content: t('删除后不可恢复，请确认该邀请码不再使用。'),
      onOk: async () => {
        setLoading(true);
        try {
          const res = await API.delete(`/api/invitation_code/${record.id}`);
          const { success, message } = res.data;
          if (success) {
            showSuccess(t('操作成功完成！'));
            await refresh();
          } else {
            showError(message);
          }
        } catch (error) {
          showError(error.message);
        } finally {
          setLoading(false);
        }
      },
    });
  };

  const clearInvalidInvitationCodes = async () => {
    Modal.confirm({
      title: t('确定清除所有失效邀请码？'),
      content: t('将删除已禁用、已过期或已用尽的邀请码，此操作不可撤销。'),
      onOk: async () => {
        setLoading(true);
        try {
          const res = await API.delete('/api/invitation_code/invalid');
          const { success, message, data } = res.data;
          if (success) {
            showSuccess(t('已删除 {{count}} 条失效邀请码', { count: data }));
            await refresh(1);
          } else {
            showError(message);
          }
        } catch (error) {
          showError(error.message);
        } finally {
          setLoading(false);
        }
      },
    });
  };

  return (
    <>
      <EditInvitationCodeModal
        refresh={refresh}
        editingInvitationCode={editingInvitationCode}
        visible={showEdit}
        handleClose={closeEditModal}
      />

      <CardPro
        type='type1'
        descriptionArea={
          <div className='flex items-center justify-between gap-3'>
            <div>
              <Text strong>{t('邀请码管理')}</Text>
              <div className='text-xs text-gray-500'>
                {t('管理注册邀请码、查看使用记录与生命周期状态')}
              </div>
            </div>
            <Button
              type='tertiary'
              theme='outline'
              size='small'
              onClick={() => setCompactMode((prev) => !prev)}
            >
              {compactMode ? t('标准视图') : t('紧凑视图')}
            </Button>
          </div>
        }
        actionsArea={
          <div className='flex flex-col lg:flex-row justify-between items-center gap-2 w-full'>
            <div className='flex flex-wrap gap-2 w-full lg:w-auto'>
              <Button type='primary' onClick={openCreateModal}>
                {t('创建邀请码')}
              </Button>
              <Button onClick={() => refresh()}>{t('刷新')}</Button>
              <Button
                type='danger'
                theme='light'
                onClick={clearInvalidInvitationCodes}
              >
                {t('清除失效邀请码')}
              </Button>
            </div>
            <div className='w-full lg:w-80'>
              <Input
                value={keyword}
                onChange={setKeyword}
                onEnterPress={handleSearch}
                placeholder={t('搜索名称、邀请码、创建者或所有者')}
                suffix={
                  <Button
                    type='primary'
                    size='small'
                    loading={searching}
                    onClick={handleSearch}
                  >
                    {t('搜索')}
                  </Button>
                }
              />
            </div>
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: activePage,
          pageSize,
          total,
          onPageChange: handlePageChange,
          onPageSizeChange: handlePageSizeChange,
          isMobile,
          t,
        })}
        t={t}
      >
        <InvitationCodesTable
          invitationCodes={invitationCodes}
          loading={loading}
          compactMode={compactMode}
          activePage={activePage}
          pageSize={pageSize}
          total={total}
          handlePageChange={handlePageChange}
          handlePageSizeChange={handlePageSizeChange}
          copyText={copyText}
          onEdit={openEditModal}
          onDelete={deleteInvitationCode}
          onToggleStatus={updateInvitationCodeStatus}
          t={t}
        />
      </CardPro>
    </>
  );
};

export default InvitationCodesPage;

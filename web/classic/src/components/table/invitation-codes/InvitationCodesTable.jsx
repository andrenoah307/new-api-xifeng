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

import React, { useMemo, useState } from 'react';
import { Empty, Modal, Typography } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import CardTable from '../../common/ui/CardTable';
import { API, showError, timestamp2string } from '../../../helpers';
import { getInvitationCodesColumns } from './InvitationCodesColumnDefs';

const { Text } = Typography;

const InvitationCodesTable = ({
  invitationCodes,
  loading,
  compactMode,
  activePage,
  pageSize,
  total,
  handlePageChange,
  handlePageSizeChange,
  copyText,
  onEdit,
  onDelete,
  onToggleStatus,
  t,
}) => {
  const [showUsagesModal, setShowUsagesModal] = useState(false);
  const [usageLoading, setUsageLoading] = useState(false);
  const [currentRecord, setCurrentRecord] = useState(null);
  const [usages, setUsages] = useState([]);

  const handleViewUsages = async (record) => {
    setCurrentRecord(record);
    setShowUsagesModal(true);
    setUsageLoading(true);
    try {
      const res = await API.get(`/api/invitation_code/${record.id}/usages`);
      const { success, message, data } = res.data;
      if (success) {
        setUsages(data || []);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setUsageLoading(false);
    }
  };

  const columns = useMemo(
    () =>
      getInvitationCodesColumns({
        t,
        copyText,
        onViewUsages: handleViewUsages,
        onEdit,
        onDelete,
        onToggleStatus,
      }),
    [t, copyText, onEdit, onDelete, onToggleStatus],
  );

  const tableColumns = useMemo(() => {
    return compactMode
      ? columns.map((column) => {
          if (column.dataIndex === 'operate') {
            const { fixed, ...rest } = column;
            return rest;
          }
          return column;
        })
      : columns;
  }, [columns, compactMode]);

  const usageColumns = useMemo(
    () => [
      {
        title: t('用户ID'),
        dataIndex: 'user_id',
      },
      {
        title: t('用户名'),
        dataIndex: 'username',
        render: (text) => text || '-',
      },
      {
        title: t('使用时间'),
        dataIndex: 'used_time',
        render: (text) => timestamp2string(text),
      },
    ],
    [t],
  );

  return (
    <>
      <CardTable
        columns={tableColumns}
        dataSource={invitationCodes}
        scroll={compactMode ? undefined : { x: 'max-content' }}
        pagination={{
          currentPage: activePage,
          pageSize,
          total,
          showSizeChanger: true,
          pageSizeOptions: [10, 20, 50, 100],
          onPageChange: handlePageChange,
          onPageSizeChange: handlePageSizeChange,
        }}
        hidePagination={true}
        loading={loading}
        rowKey='id'
        empty={
          <Empty
            image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
            }
            description={t('搜索无结果')}
            style={{ padding: 30 }}
          />
        }
        className='rounded-xl overflow-hidden'
        size='middle'
      />

      <Modal
        title={t('邀请码使用记录')}
        visible={showUsagesModal}
        onCancel={() => {
          setShowUsagesModal(false);
          setCurrentRecord(null);
          setUsages([]);
        }}
        footer={null}
        width={720}
      >
        <div className='mb-3'>
          <Text type='secondary'>
            {currentRecord
              ? t('邀请码：{{code}}', { code: currentRecord.code })
              : ''}
          </Text>
        </div>
        <CardTable
          columns={usageColumns}
          dataSource={usages}
          rowKey='id'
          loading={usageLoading}
          pagination={false}
          empty={
            <Empty
              image={<IllustrationNoResult style={{ width: 120, height: 120 }} />}
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 120, height: 120 }} />
              }
              description={t('暂无使用记录')}
              style={{ padding: 12 }}
            />
          }
        />
      </Modal>
    </>
  );
};

export default InvitationCodesTable;

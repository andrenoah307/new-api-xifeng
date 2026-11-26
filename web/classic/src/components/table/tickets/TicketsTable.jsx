import React, { useMemo } from 'react';
import { Checkbox, Empty } from '@douyinfe/semi-ui';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import CardTable from '../../common/ui/CardTable';
import { getTicketsColumns } from './TicketsColumnDefs';

const TicketsTable = ({
  tickets = [],
  loading = false,
  compactMode = false,
  activePage,
  pageSize,
  ticketCount,
  handlePageChange,
  handlePageSizeChange,
  admin = false,
  onOpenDetail,
  onCloseTicket,
  staffIndex, // Map<number, TicketStaffUser>，用于"分配客服"列渲染
  showAssignee = false, // 仅管理员及以上视角展示"分配客服"列
  rowSelection,
  t,
}) => {
  const isMobile = useIsMobile();
  const columns = useMemo(
    () =>
      getTicketsColumns({
        t,
        admin,
        onOpenDetail,
        onCloseTicket,
        staffIndex,
        showAssignee,
      }),
    [t, admin, onOpenDetail, onCloseTicket, staffIndex, showAssignee],
  );
  const displayColumns = [...columns];
  if (isMobile && rowSelection) {
    displayColumns.unshift({
      key: 'selection',
      title: '',
      render: (_, record) => (
        <Checkbox
          aria-label={t('Select ticket {{id}}', { id: record.id })}
          checked={rowSelection.selectedRowKeys.includes(record.id)}
          disabled={rowSelection.getCheckboxProps?.(record).disabled}
          onChange={(event) =>
            rowSelection.onChange(
              event.target.checked
                ? [...rowSelection.selectedRowKeys, record.id]
                : rowSelection.selectedRowKeys.filter((id) => id !== record.id),
            )
          }
        />
      ),
    });
  }

  return (
    <CardTable
      rowKey='id'
      rowSelection={rowSelection}
      columns={displayColumns}
      dataSource={tickets}
      loading={loading}
      hidePagination
      scroll={undefined}
      pagination={{
        currentPage: activePage,
        pageSize,
        total: ticketCount,
        pageSizeOpts: [10, 20, 50, 100],
        showSizeChanger: true,
        onPageSizeChange: handlePageSizeChange,
        onPageChange: handlePageChange,
      }}
      onRow={(record) => ({
        onClick: () => onOpenDetail?.(record),
      })}
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('暂无工单')}
          style={{ padding: 30 }}
        />
      }
      className='overflow-hidden'
    />
  );
};

export default TicketsTable;

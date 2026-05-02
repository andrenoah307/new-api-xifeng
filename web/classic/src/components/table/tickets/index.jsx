import React from 'react';
import { Typography } from '@douyinfe/semi-ui';
import CardPro from '../../common/ui/CardPro';
import CompactModeToggle from '../../common/ui/CompactModeToggle';
import TicketsTable from './TicketsTable';
import { createCardProPagination } from '../../../helpers/utils';
import { useIsMobile } from '../../../hooks/common/useIsMobile';

const { Text, Title } = Typography;

const TicketsPage = ({
  title,
  description,
  actionsArea,
  compactMode,
  setCompactMode,
  tickets,
  loading,
  activePage,
  pageSize,
  ticketCount,
  handlePageChange,
  handlePageSizeChange,
  admin = false,
  onOpenDetail,
  onCloseTicket,
  staffIndex,
  showAssignee = false,
  t,
}) => {
  const isMobile = useIsMobile();

  return (
    <CardPro
      type='type1'
      descriptionArea={
        <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3 w-full'>
          <div className='flex flex-col gap-1'>
            <Title heading={6} className='!mb-0'>
              {title}
            </Title>
            <Text type='secondary'>{description}</Text>
          </div>
          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        </div>
      }
      actionsArea={actionsArea}
      paginationArea={createCardProPagination({
        currentPage: activePage,
        pageSize,
        total: ticketCount,
        onPageChange: handlePageChange,
        onPageSizeChange: handlePageSizeChange,
        isMobile,
        t,
      })}
      t={t}
    >
      <TicketsTable
        tickets={tickets}
        loading={loading}
        compactMode={compactMode}
        activePage={activePage}
        pageSize={pageSize}
        ticketCount={ticketCount}
        handlePageChange={handlePageChange}
        handlePageSizeChange={handlePageSizeChange}
        admin={admin}
        onOpenDetail={onOpenDetail}
        onCloseTicket={onCloseTicket}
        staffIndex={staffIndex}
        showAssignee={showAssignee}
        t={t}
      />
    </CardPro>
  );
};

export default TicketsPage;


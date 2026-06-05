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

import React, { useContext } from 'react';
import CardPro from '../../common/ui/CardPro';
import LogsTable from './UsageLogsTable';
import LogsActions from './UsageLogsActions';
import LogsFilters from './UsageLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import UserInfoModal from './modals/UserInfoModal';
import ChannelAffinityUsageCacheModal from './modals/ChannelAffinityUsageCacheModal';
import ParamOverrideModal from './modals/ParamOverrideModal';
import OfflineExportModal from './modals/OfflineExportModal';
import ExportTasksModal from './modals/ExportTasksModal';
import { useLogsData } from '../../../hooks/usage-logs/useUsageLogsData';
import { useOfflineExport } from '../../../hooks/usage-logs/useOfflineExport';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import { StatusContext } from '../../../context/Status';
import { UserContext } from '../../../context/User';

const LogsPage = () => {
  const logsData = useLogsData();
  const isMobile = useIsMobile();
  const [statusState] = useContext(StatusContext);
  const [userState] = useContext(UserContext);
  const offlineExportEnabled = !!statusState?.status?.enable_log_export_offline;

  const offlineExport = useOfflineExport();

  // Build filters from current form state for offline export
  const buildOfflineFilters = () => {
    const formValues = logsData.formApi ? logsData.formApi.getValues() : {};
    const dateRange = formValues.dateRange || [];
    return {
      start_timestamp: dateRange[0] ? Math.floor(Date.parse(dateRange[0]) / 1000) : undefined,
      end_timestamp: dateRange[1] ? Math.floor(Date.parse(dateRange[1]) / 1000) : undefined,
      model_name: formValues.model_name || undefined,
      token_name: formValues.token_name || undefined,
      channel: formValues.channel || undefined,
    };
  };

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...logsData} />
      <UserInfoModal {...logsData} />
      <ChannelAffinityUsageCacheModal {...logsData} />
      <ParamOverrideModal {...logsData} />
      <OfflineExportModal
        visible={offlineExport.modalOpen}
        onClose={() => offlineExport.setModalOpen(false)}
        onSubmit={offlineExport.submitOfflineExport}
        submitting={offlineExport.submitting}
        filters={buildOfflineFilters()}
        userEmail={userState?.user?.email || ''}
      />
      <ExportTasksModal
        visible={offlineExport.tasksModalOpen}
        onClose={() => offlineExport.setTasksModalOpen(false)}
        tasks={offlineExport.tasks}
        total={offlineExport.tasksTotal}
        page={offlineExport.tasksPage}
        loading={offlineExport.tasksLoading}
        onPageChange={(p) => offlineExport.fetchTasks(p)}
        onRefresh={offlineExport.fetchTasks}
      />

      {/* Main Content */}
      <CardPro
        type='type2'
        statsArea={<LogsActions {...logsData} />}
        searchArea={
          <LogsFilters
            {...logsData}
            offlineExportEnabled={offlineExportEnabled}
            onOfflineExport={() => offlineExport.setModalOpen(true)}
            onShowExportTasks={() => offlineExport.setTasksModalOpen(true)}
          />
        }
        paginationArea={createCardProPagination({
          currentPage: logsData.activePage,
          pageSize: logsData.pageSize,
          total: logsData.logCount,
          onPageChange: logsData.handlePageChange,
          onPageSizeChange: logsData.handlePageSizeChange,
          isMobile: isMobile,
          isAdmin: logsData.isAdminUser,
          t: logsData.t,
        })}
        t={logsData.t}
      >
        <LogsTable {...logsData} />
      </CardPro>
    </>
  );
};

export default LogsPage;

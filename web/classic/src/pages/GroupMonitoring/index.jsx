import React, { lazy, Suspense } from 'react';
import Loading from '../../components/common/ui/Loading';

const GroupMonitoringDashboard = lazy(() =>
  import('../../components/monitoring/GroupMonitoringDashboard')
);

const GroupMonitoring = () => (
  <Suspense fallback={<Loading />}>
    <GroupMonitoringDashboard />
  </Suspense>
);

export default GroupMonitoring;

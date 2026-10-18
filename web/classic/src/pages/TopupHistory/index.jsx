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

import React, { lazy, Suspense, useContext } from 'react';
import { Tabs, TabPane, Typography } from '@douyinfe/semi-ui';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../context/User';
import TopupHistoryPage from '../../components/topup/TopupHistoryPage';

const FinanceRecords = lazy(() => import('../Finance'));

export default function TopupHistory() {
  const { t } = useTranslation();
  const [state] = useContext(UserContext);
  const user = state.user;
  const [params, setParams] = useSearchParams();
  const canRead =
    user?.role === 100 ||
    user?.permissions?.admin_permissions?.finance?.read === true;
  const requested = params.get('view');
  const finance =
    canRead && (requested === 'finance' || (!requested && user?.role < 10));
  return (
    <div className='mt-[60px] p-4 md:p-6 space-y-4'>
      <Typography.Title heading={3}>{t('充值账单')}</Typography.Title>
      {canRead && (
        <Tabs
          activeKey={finance ? 'finance' : 'records'}
          onChange={(view) => setParams({ view })}
        >
          <TabPane tab={t('充值账单')} itemKey='records' />
          <TabPane tab={t('Finance')} itemKey='finance' />
        </Tabs>
      )}
      {finance ? (
        <Suspense fallback={<p>{t('加载中...')}</p>}>
          <FinanceRecords key={user?.id} />
        </Suspense>
      ) : (
        <TopupHistoryPage key={`${user?.id}-${user?.role}`} />
      )}
    </div>
  );
}

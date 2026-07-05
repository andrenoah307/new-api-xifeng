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

import React from 'react';
import { Modal, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

// AccountRiskWarningModal is shown to a user the first time they open the
// dashboard after the risk control engine has marked their account.
//
// The wording is intentionally vague — we never expose the underlying rule
// name, scope, group, or timestamp — so users cannot reverse-engineer
// detection thresholds. Acknowledging only clears the modal trigger; the
// actual block (if any) is unaffected.
export default function AccountRiskWarningModal({ visible, onAcknowledge }) {
  const { t } = useTranslation();
  return (
    <Modal
      title={t('账户风险提示')}
      visible={visible}
      onOk={onAcknowledge}
      okText={t('我已知晓')}
      footer={null}
      maskClosable={false}
      closable={false}
      centered
      width={520}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
    >
      <div style={{ padding: '4px 0 8px' }}>
        <Text>
          {t(
            '您的账户最近触发了平台风险防护策略，部分功能可能受到限制。如有疑问请联系管理员。',
          )}
        </Text>
      </div>
      <div
        style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}
      >
        {/* Single button — Modal.footer = null + custom button keeps the
            layout consistent with the rest of the admin UX while preventing
            accidental dismissal via mask click. */}
        <button
          type='button'
          className='semi-button semi-button-primary'
          onClick={onAcknowledge}
          style={{
            padding: '6px 18px',
            borderRadius: 6,
            border: 'none',
            background: 'var(--semi-color-primary)',
            color: '#fff',
            cursor: 'pointer',
          }}
        >
          {t('我已知晓')}
        </button>
      </div>
    </Modal>
  );
}

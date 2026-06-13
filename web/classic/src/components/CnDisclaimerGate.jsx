import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { Button, Checkbox, Modal } from '@douyinfe/semi-ui';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';
import { useTranslation } from 'react-i18next';
import { API } from '../helpers';
import { StatusContext } from '../context/Status';
import {
  isStillSilent,
  markAcknowledged,
} from '../helpers/cnDisclaimerStorage';

const ROUTE_EXEMPTIONS = ['/setup'];

const CnDisclaimerGate = () => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const location = useLocation();
  const required = Boolean(statusState?.status?.cn_disclaimer_required);
  const exempted = ROUTE_EXEMPTIONS.some((p) =>
    (location?.pathname || '').startsWith(p),
  );

  const [data, setData] = useState(null);
  const [agreed, setAgreed] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);

  useEffect(() => {
    if (!required || exempted) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get('/api/cn-disclaimer');
        if (cancelled) return;
        const body = res?.data || {};
        setData(body);
        const hash = body.hash || '';
        const silence = Number(body.silence_minutes ?? 0);
        setAcknowledged(isStillSilent(hash, silence));
      } catch {
        // network errors leave Gate dormant; admin can retry on next page load
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [required, exempted]);

  const visible = useMemo(() => {
    if (!required || exempted) return false;
    if (!data) return false;
    if (!data.enabled) return false;
    if (!data.content || !data.hash) return false;
    if (acknowledged) return false;
    return true;
  }, [required, exempted, data, acknowledged]);

  if (!visible) return null;

  const handleConfirm = () => {
    if (!agreed || !data?.hash) return;
    markAcknowledged(data.hash);
    setAcknowledged(true);
  };

  return (
    <Modal
      title={data.title}
      visible={visible}
      maskClosable={false}
      closable={false}
      escClose={false}
      centered={true}
      width={640}
      style={{ maxWidth: '92vw' }}
      bodyStyle={{
        maxHeight: 'calc(80vh - 120px)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
      getPopupContainer={() => document.body}
      footer={
        <div className='flex items-center justify-between gap-3'>
          <Checkbox
            checked={agreed}
            onChange={(e) => setAgreed(e.target.checked)}
          >
            <Text size='small' className='text-gray-600'>
              {t('我已阅读并知悉')}
            </Text>
          </Checkbox>
          <Button
            theme='solid'
            type='primary'
            disabled={!agreed}
            onClick={handleConfirm}
          >
            {t('我已确认')}
          </Button>
        </div>
      }
    >
      <div
        className='text-sm leading-6 whitespace-pre-wrap break-words'
        style={{ wordBreak: 'break-word' }}
      >
        {data.content}
      </div>
    </Modal>
  );
};

export default CnDisclaimerGate;

/*
Copyright (C) 2023-2026 QuantumNous

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
import React, { useEffect, useState } from 'react';
import { Button, InputNumber, Select, Typography } from '@douyinfe/semi-ui';
import { IconSave } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, isRoot, showError, showSuccess } from '../../../../helpers';

export default function ChannelCostSettings({ channelId }) {
  const { t } = useTranslation();
  const [settings, setSettings] = useState(null);
  const [loading, setLoading] = useState(false);
  const root = isRoot();
  useEffect(() => {
    let active = true;
    setSettings(null);
    if (!root) return;
    API.get(`/api/channel-cost/${channelId}`)
      .then(({ data }) => {
        if (active && data.success) setSettings(data.data);
      })
      .catch(showError);
    return () => {
      active = false;
    };
  }, [channelId, root]);
  if (!root || !settings) return null;
  const save = async () => {
    setLoading(true);
    try {
      const { data } = await API.put(
        `/api/channel-cost/${channelId}`,
        settings,
      );
      if (data.success) {
        setSettings(data.data);
        showSuccess(t('Saved successfully'));
      } else showError(data.message);
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };
  return (
    <section className='grid gap-3 border-b pb-4 mb-4'>
      <Typography.Title heading={6}>{t('Channel cost')}</Typography.Title>
      <label>{t('Cost source')}</label>
      <Select
        aria-label={t('Cost source')}
        value={settings.mode || 'disabled'}
        onChange={(mode) => setSettings({ ...settings, mode })}
      >
        <Select.Option value='disabled'>{t('Disabled')}</Select.Option>
        <Select.Option value='manual'>{t('Manual')}</Select.Option>
        <Select.Option value='auto'>
          Sub2API - {t('Upstream declared multiplier')}
        </Select.Option>
      </Select>
      {settings.mode === 'manual' && (
        <>
          <label>{t('Cost multiplier')}</label>
          <InputNumber
            aria-label={t('Cost multiplier')}
            min={0}
            max={10000}
            value={settings.ratio ?? ''}
            onChange={(ratio) =>
              setSettings({
                ...settings,
                ratio: typeof ratio === 'number' ? ratio : null,
              })
            }
          />
        </>
      )}
      {settings.mode === 'auto' && (
        <>
          <Typography.Text>
            {t('Cost multiplier')}: {settings.ratio ?? '-'}
          </Typography.Text>
          {settings.sync_status === 'failed' && (
            <Typography.Text type='danger'>
              {t('Failed to load')}
            </Typography.Text>
          )}
        </>
      )}
      <Button
        htmlType='button'
        icon={<IconSave />}
        loading={loading}
        disabled={
          settings.mode === 'manual' &&
          (settings.ratio == null ||
            !Number.isFinite(settings.ratio) ||
            settings.ratio < 0 ||
            settings.ratio > 10000)
        }
        onClick={save}
      >
        {t('Save cost settings')}
      </Button>
    </section>
  );
}

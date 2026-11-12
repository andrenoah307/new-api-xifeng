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
import React, { useEffect, useRef, useState } from 'react';
import { Button, Form, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { parseUpstreamErrorMessages } from '../../../../../shared/upstream-error-messages';

export default function SettingsUpstreamErrors({ options, refresh }) {
  const { t } = useTranslation();
  const form = useRef();
  const saving = useRef(false);
  const [loading, setLoading] = useState(false);
  const value = options.UpstreamErrorMessages;
  useEffect(() => {
    form.current?.setValues(parseUpstreamErrorMessages(value));
  }, [value]);
  const labels = {
    client: t('Client errors (4xx)'),
    server: t('Server errors (5xx or unknown)'),
  };

  async function save(values) {
    if (saving.current) return;
    saving.current = true;
    setLoading(true);
    try {
      const { data } = await API.put('/api/option/', {
        key: 'UpstreamErrorMessages',
        value: JSON.stringify(values),
      });
      if (!data.success) {
        showError(data.message);
        return;
      }
      showSuccess(t('Saved successfully'));
      await refresh();
    } catch {
      showError(t('保存失败，请重试'));
    } finally {
      saving.current = false;
      setLoading(false);
    }
  }

  return (
    <section>
      <Typography.Title heading={5}>
        {t('Upstream Error Redaction')}
      </Typography.Title>
      <Form
        initValues={parseUpstreamErrorMessages(value)}
        getFormApi={(api) => {
          form.current = api;
        }}
        onSubmit={save}
      >
        {Object.entries(labels).map(([key, label]) => (
          <Form.Input
            key={key}
            field={key}
            label={label}
            maxLength={300}
            disabled={loading}
            rules={[{ required: true }]}
          />
        ))}
        <Button htmlType='submit' loading={loading} disabled={loading}>
          {t('保存')}
        </Button>
      </Form>
    </section>
  );
}

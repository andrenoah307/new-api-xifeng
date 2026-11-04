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
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Input,
  Select,
  Switch,
  Checkbox,
  Toast,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/api';
import {
  captchaFields,
  captchaScenes,
  captchaSecretFields,
} from '../../../../shared/captcha-settings';

export default function CaptchaSettings() {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState(null);
  const [config, setConfig] = useState(null);
  const [cleared, setCleared] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  useEffect(() => {
    let active = true;
    API.get('/api/captcha/settings')
      .then(({ data }) => {
        if (!active) return;
        if (!data.success) {
          setError(true);
          return;
        }
        setSnapshot(data.data);
        setConfig(data.data.config);
      })
      .catch(() => {
        if (active) setError(true);
      });
    return () => {
      active = false;
    };
  }, []);
  const save = async () => {
    setBusy(true);
    try {
      const { data } = await API.put('/api/captcha/settings', {
        config,
        revision: snapshot.revision,
        clear_secrets: cleared,
      });
      if (!data.success) {
        Toast.error(data.message);
        return;
      }
      setSnapshot(data.data);
      setConfig(data.data.config);
      setCleared([]);
      Toast.success(t('Saved successfully'));
    } finally {
      setBusy(false);
    }
  };
  if (error) return <p role='alert'>{t('Failed to load settings')}</p>;
  if (!config) return <p role='status'>{t('Loading...')}</p>;
  return (
    <section style={{ padding: 16 }}>
      <h3 style={{ marginBottom: 16 }}>{t('Bot Protection')}</h3>
      <fieldset disabled={busy} style={{ border: 0, padding: 0, minWidth: 0 }}>
        <label
          style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}
        >
          {t('Enable captcha protection')}
          <Switch
            disabled={busy}
            checked={config.enabled}
            onChange={(enabled) => setConfig({ ...config, enabled })}
          />
        </label>
        {['turnstile', 'tencent'].map((provider) => (
          <section
            key={provider}
            style={{
              borderTop: '1px solid var(--semi-color-border)',
              marginTop: 20,
              paddingTop: 16,
            }}
          >
            <h4>
              {provider === 'turnstile'
                ? 'Cloudflare Turnstile'
                : t('Tencent Cloud Captcha')}
            </h4>
            <div
              style={{
                display: 'grid',
                gridTemplateColumns:
                  'repeat(auto-fit, minmax(min(240px, 100%), 1fr))',
                gap: 16,
                marginTop: 12,
              }}
            >
              {Object.entries(captchaFields)
                .filter(([key]) => key.startsWith(provider))
                .map(([key, label]) => {
                  const secret = captchaSecretFields.includes(key);
                  return (
                    <div key={key}>
                      <label htmlFor={`captcha-${key}`}>{t(label)}</label>
                      <Input
                        id={`captcha-${key}`}
                        type={secret ? 'password' : 'text'}
                        autoComplete={secret ? 'new-password' : 'off'}
                        maxLength={2048}
                        disabled={busy || cleared.includes(key)}
                        value={config[key]}
                        placeholder={
                          secret && snapshot.configured[key]
                            ? t('Configured; leave blank to keep')
                            : ''
                        }
                        onChange={(value) =>
                          setConfig({ ...config, [key]: value })
                        }
                      />
                      {secret && (
                        <Checkbox
                          disabled={busy}
                          checked={cleared.includes(key)}
                          onChange={(event) =>
                            setCleared((old) =>
                              event.target.checked
                                ? [...old, key]
                                : old.filter((item) => item !== key),
                            )
                          }
                        >
                          {t('Clear saved secret')}
                        </Checkbox>
                      )}
                    </div>
                  );
                })}
            </div>
            {provider === 'tencent' && (
              <label
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  gap: 12,
                  marginTop: 16,
                }}
              >
                {t('CaptchaAppId authentication')}
                <Switch
                  disabled={busy}
                  checked={config.tencent_app_id_auth}
                  onChange={(value) =>
                    setConfig({ ...config, tencent_app_id_auth: value })
                  }
                />
              </label>
            )}
          </section>
        ))}
        <h4 style={{ margin: '20px 0 12px' }}>{t('Protected actions')}</h4>
        {Object.entries(captchaScenes).map(([scene, label]) => (
          <div
            key={scene}
            style={{
              display: 'grid',
              gridTemplateColumns: 'minmax(0, 1fr) 160px',
              alignItems: 'center',
              gap: 12,
              marginBottom: 10,
            }}
          >
            <label htmlFor={`captcha-scene-${scene}`}>{t(label)}</label>
            <Select
              disabled={busy}
              id={`captcha-scene-${scene}`}
              aria-label={t(label)}
              value={config.scenes[scene] || 'off'}
              onChange={(value) =>
                setConfig({
                  ...config,
                  scenes: { ...config.scenes, [scene]: value },
                })
              }
              optionList={[
                { value: 'off', label: t('Disabled') },
                { value: 'turnstile', label: 'Turnstile' },
                { value: 'tencent', label: 'Tencent' },
              ]}
            />
          </div>
        ))}
        <Button htmlType='button' theme='solid' onClick={save} loading={busy}>
          {t('Save')}
        </Button>
      </fieldset>
    </section>
  );
}

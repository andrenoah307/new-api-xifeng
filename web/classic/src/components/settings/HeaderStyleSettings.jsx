import React, { useContext, useState } from 'react';
import { Button, Select } from '@douyinfe/semi-ui';
import { Sparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { StatusContext } from '../../context/Status';

export default function HeaderStyleSettings({ value }) {
  const { t } = useTranslation();
  const [, dispatch] = useContext(StatusContext);
  const initial = value === 'packy' ? 'packy' : 'default';
  const [style, setStyle] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [saving, setSaving] = useState(false);
  const save = async () => {
    if (saving) return;
    setSaving(true);
    try {
      const response = await API.put('/api/option/', { key: 'HeaderNavStyle', value: style });
      if (!response.data?.success) throw new Error(response.data?.message || t('保存失败'));
      setSaved(style);
      const status = await API.get('/api/status');
      if (status.data?.success) {
        dispatch({ type: 'set', payload: status.data.data });
        localStorage.setItem('status', JSON.stringify(status.data.data));
      }
      showSuccess(t('保存成功'));
    } catch (error) {
      showError(error.message || t('请求失败'));
    } finally {
      setSaving(false);
    }
  };
  return (
    <section className='mb-6 border-b pb-6'>
      <h3 id='header-style-label' className='mb-4 text-base font-semibold'>{t('Top navigation style')}</h3>
      <div className='flex flex-wrap items-center gap-3'>
        <Select aria-labelledby='header-style-label' value={style} disabled={saving} onChange={setStyle}
          style={{ minWidth: 200, maxWidth: '100%' }}
          optionList={[{ value: 'default', label: t('Default style') }, { value: 'packy', label: t('Packy style') }]} />
        <Button theme='solid' loading={saving} disabled={style === saved} onClick={save}>{t('保存设置')}</Button>
      </div>
      <div className='mt-4 overflow-hidden border-y py-4' aria-label={t('预览')}>
        <nav data-header-style={style} data-header-links className='flex flex-wrap gap-2 text-sm'>
          <a href='#' aria-current='page' onClick={(event) => event.preventDefault()}><Sparkles className='header-nav-symbol' aria-hidden='true' />{t('首页')}</a>
          <a href='#' onClick={(event) => event.preventDefault()}><Sparkles className='header-nav-symbol' aria-hidden='true' />{t('模型广场')}</a>
          <a href='#' onClick={(event) => event.preventDefault()}><Sparkles className='header-nav-symbol' aria-hidden='true' />{t('文档')}</a>
        </nav>
      </div>
    </section>
  );
}

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

import React, { useEffect, useState, useContext, useRef, useMemo } from 'react';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
  renderGroupOption,
  getCurrencyConfig,
  getModelCategories,
  selectFilter,
} from '../../../../helpers';
import {
  quotaToDisplayAmount,
  quotaToDisplayInputAmount,
  displayAmountToQuota,
  getPeriodConversionConfig,
} from '../../../../helpers/quota';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Avatar,
  Form,
  Col,
  Row,
  InputNumber,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconLink,
  IconSave,
  IconClose,
  IconKey,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../../../context/Status';
import {
  TOKEN_PERIOD_MAX_DAYS,
  convertPeriodLimitUnit,
  formatPeriodResetAt,
  getPeriodResetAt,
  periodFormToPayload,
  periodResponseToForm,
  validatePeriodForm,
  amountToCanonicalQuota,
  isPositiveIntegerString,
} from '../token-period';

const { Text, Title } = Typography;

const EditTokenModal = (props) => {
  const { t, i18n } = useTranslation();
  const [statusState, statusDispatch] = useContext(StatusContext);
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [showQuotaInput, setShowQuotaInput] = useState(false);
  const periodCanonicalQuotaRef = useRef(null);
  const periodAnchorAtRef = useRef(0);
  const isEdit = props.editingToken.id !== undefined;

  // 周期限额金额与令牌额度同口径：都走管理员配置的站点展示币种与汇率。
  // statusState 变化即代表管理员改了汇率/币种，重新取一次配置。
  const periodConversion = useMemo(
    () => getPeriodConversionConfig(),
    [statusState?.status],
  );
  // TOKENS 口径没有货币符号，金额与原生额度同刻度，标签与占位符跟着走
  const periodAmountPlaceholder = periodConversion.symbol
    ? '10.00'
    : String(Math.max(1, Math.trunc(periodConversion.quotaPerUnit)));
  const periodAmountUnitLabel = periodConversion.symbol
    ? t('金额（{{symbol}}）', { symbol: periodConversion.symbol })
    : t('金额');

  const getInitValues = () => ({
    name: '',
    remain_quota: 0,
    remain_amount: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: [],
    allow_ips: '',
    group: '',
    cross_group_retry: false,
    tokenCount: 1,
    period_enabled: false,
    period_type: '',
    period_days: 0,
    period_limit_unit: 'cny',
    period_limit_value: '0',
    period_reset_at: 0,
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (!formApiRef.current) return;
    if (seconds !== 0) {
      timestamp += seconds;
      formApiRef.current.setValue('expired_time', timestamp2string(timestamp));
    } else {
      formApiRef.current.setValue('expired_time', -1);
    }
  };

  const loadModels = async () => {
    let res = await API.get(`/api/user/models`);
    const { success, message, data } = res.data;
    if (success) {
      const categories = getModelCategories(t);
      let localModelOptions = data.map((model) => {
        let icon = null;
        for (const [key, category] of Object.entries(categories)) {
          if (key !== 'all' && category.filter({ model_name: model })) {
            icon = category.icon;
            break;
          }
        }
        return {
          label: (
            <span className='flex items-center gap-1'>
              {icon}
              {model}
            </span>
          ),
          value: model,
        };
      });
      setModels(localModelOptions);
    } else {
      showError(t(message));
    }
  };

  const loadGroups = async () => {
    let res = await API.get(`/api/user/self/groups`);
    const { success, message, data } = res.data;
    if (success) {
      const regionBlockedGroups = statusState?.status?.region_blocked_groups || [];
      let localGroupOptions = Object.entries(data)
        .filter(([group]) => !regionBlockedGroups.includes(group))
        .map(([group, info]) => ({
        label: info.desc,
        value: group,
        ratio: info.ratio,
      }));
      if (statusState?.status?.default_use_auto_group) {
        if (localGroupOptions.some((group) => group.value === 'auto')) {
          localGroupOptions.sort((a, b) => (a.value === 'auto' ? -1 : 1));
        }
      }
      setGroups(localGroupOptions);
      // if (statusState?.status?.default_use_auto_group && formApiRef.current) {
      //   formApiRef.current.setValue('group', 'auto');
      // }
    } else {
      showError(t(message));
    }
  };

  const loadToken = async () => {
    setLoading(true);
    let res = await API.get(`/api/token/${props.editingToken.id}`);
    const { success, message, data } = res.data;
    if (success) {
      const periodValues = periodResponseToForm(data, periodConversion);
      periodCanonicalQuotaRef.current = periodValues.canonicalQuota;
      periodAnchorAtRef.current = periodValues.period_anchor_at;
      const expiredTime =
        data.expired_time === -1
          ? -1
          : timestamp2string(data.expired_time);
      const modelLimits = data.model_limits
        ? data.model_limits.split(',').filter(Boolean)
        : [];
      if (formApiRef.current) {
        formApiRef.current.setValues({
          ...getInitValues(),
          name: data.name || '',
          remain_quota: Number(data.remain_quota) || 0,
          remain_amount: quotaToDisplayInputAmount(data.remain_quota || 0),
          expired_time: expiredTime,
          unlimited_quota: Boolean(data.unlimited_quota),
          model_limits_enabled: Boolean(data.model_limits_enabled),
          model_limits: modelLimits,
          allow_ips: data.allow_ips || '',
          group: data.group || '',
          cross_group_retry: Boolean(data.cross_group_retry),
          ...periodValues,
        });
      }
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    if (formApiRef.current) {
      if (!isEdit) {
        periodCanonicalQuotaRef.current = null;
        periodAnchorAtRef.current = 0;
        formApiRef.current.setValues(getInitValues());
      }
    }
    loadModels();
    loadGroups();
  }, [props.editingToken.id]);

  useEffect(() => {
    if (props.visiable) {
      if (isEdit) {
        loadToken();
      } else {
        periodCanonicalQuotaRef.current = null;
        periodAnchorAtRef.current = 0;
        formApiRef.current?.setValues(getInitValues());
      }
    } else {
      periodCanonicalQuotaRef.current = null;
      periodAnchorAtRef.current = 0;
      formApiRef.current?.reset();
    }
  }, [props.visiable, props.editingToken.id]);

  const generateRandomSuffix = () => {
    const characters =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 6; i++) {
      result += characters.charAt(
        Math.floor(Math.random() * characters.length),
      );
    }
    return result;
  };

  const getPeriodValidationMessage = (errors) => {
    if (errors.includes('period_days')) {
      return '周期天数必须在 1 到 3650 之间';
    }
    if (errors.includes('period_limit_value_integer')) {
      return '原生额度必须为正整数';
    }
    return '周期限额超出可用范围';
  };

  const buildWritableTokenInputs = (values, name) => {
    const validation = validatePeriodForm(values, periodConversion);
    if (!validation.valid) {
      return { error: getPeriodValidationMessage(validation.errors) };
    }

    let expiredTime = values.expired_time;
    if (expiredTime !== -1) {
      const time = Date.parse(expiredTime);
      if (Number.isNaN(time)) {
        return { error: '过期时间格式错误！' };
      }
      expiredTime = Math.ceil(time / 1000);
    }

    const modelLimits = Array.isArray(values.model_limits)
      ? values.model_limits.filter(Boolean)
      : String(values.model_limits || '')
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean);
    const remainQuota = values.unlimited_quota
      ? 0
      : displayAmountToQuota(values.remain_amount);
    if (!values.unlimited_quota && remainQuota <= 0) {
      return { error: '请输入金额' };
    }

    return {
      payload: {
        name,
        expired_time: expiredTime,
        remain_quota: remainQuota,
        unlimited_quota: Boolean(values.unlimited_quota),
        model_limits_enabled: modelLimits.length > 0,
        model_limits: modelLimits.join(','),
        allow_ips: values.allow_ips || '',
        group: values.group || '',
        cross_group_retry:
          values.group === 'auto' && Boolean(values.cross_group_retry),
        // This helper is the only source of period write fields. It omits
        // period_used_quota, period_start_at, period_anchor_at, and reset_at.
        ...periodFormToPayload(values),
      },
    };
  };

  const submit = async (values) => {
    setLoading(true);
    if (isEdit) {
      const name = String(values.name || '').trim();
      const built = buildWritableTokenInputs(values, name);
      if (built.error) {
        showError(t(built.error));
        setLoading(false);
        return;
      }
      const res = await API.put(`/api/token/`, {
        ...built.payload,
        id: parseInt(props.editingToken.id, 10),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('令牌更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(t(message));
      }
    } else {
      const count = parseInt(values.tokenCount, 10) || 1;
      let successCount = 0;
      for (let i = 0; i < count; i++) {
        const baseName =
          String(values.name || '').trim() === ''
            ? 'default'
            : String(values.name).trim();
        const name =
          i !== 0 || String(values.name || '').trim() === ''
            ? `${baseName}-${generateRandomSuffix()}`
            : baseName;
        const built = buildWritableTokenInputs(values, name);
        if (built.error) {
          showError(t(built.error));
          break;
        }
        const res = await API.post(`/api/token/`, built.payload);
        const { success, message } = res.data;
        if (success) {
          successCount++;
        } else {
          showError(t(message));
          break;
        }
      }
      if (successCount > 0) {
        showSuccess(t('令牌创建成功，请在列表页面点击复制获取令牌！'));
        props.refresh();
        props.handleClose();
      }
    }
    setLoading(false);
    formApiRef.current?.setValues(getInitValues());
  };

  const setPeriodEnabled = (enabled) => {
    const api = formApiRef.current;
    if (!api) return;
    api.setValue('period_enabled', Boolean(enabled));
    periodAnchorAtRef.current = 0;
    if (!enabled) {
      periodCanonicalQuotaRef.current = null;
      api.setValue('period_type', '');
      api.setValue('period_days', 0);
      api.setValue('period_limit_value', '0');
      api.setValue('period_reset_at', 0);
      return;
    }

    const current = api.getValues() || {};
    const periodUnit = current.period_limit_unit === 'quota' ? 'quota' : 'cny';
    const currentValue = String(current.period_limit_value || '').trim();
    const nextValue =
      currentValue && currentValue !== '0'
        ? currentValue
        : periodUnit === 'cny'
          ? periodAmountPlaceholder
          : String(Math.max(1, Math.trunc(periodConversion.quotaPerUnit)));
    const nextType =
      current.period_type === 'days' ||
      current.period_type === 'week' ||
      current.period_type === 'month'
        ? current.period_type
        : 'week';
    const nextDays =
      nextType === 'days' && Number.isInteger(Number(current.period_days))
        ? Number(current.period_days) || 1
        : nextType === 'days'
          ? 1
          : 0;
    periodCanonicalQuotaRef.current =
      periodUnit === 'cny'
        ? amountToCanonicalQuota(
            nextValue,
            periodConversion.displayRate,
            periodConversion.quotaPerUnit,
          )
        : isPositiveIntegerString(nextValue)
          ? Number(nextValue)
          : null;
    api.setValue('period_type', nextType);
    api.setValue('period_days', nextDays);
    api.setValue('period_limit_value', nextValue);
    api.setValue(
      'period_reset_at',
      getPeriodResetAt(
        nextType,
        nextType === 'days' ? nextDays : 0,
        Date.now(),
        0,
      ),
    );
  };

  const setPeriodType = (periodType) => {
    if (!formApiRef.current) return;
    const nextType =
      periodType === 'days' || periodType === 'week' || periodType === 'month'
        ? periodType
        : 'week';
    const currentDays = Number(formApiRef.current.getValue('period_days'));
    const nextDays =
      nextType === 'days'
        ? Number.isInteger(currentDays) && currentDays >= 1
          ? currentDays
          : 1
        : 0;
    periodAnchorAtRef.current = 0;
    formApiRef.current.setValue('period_type', nextType);
    formApiRef.current.setValue('period_days', nextDays);
    formApiRef.current.setValue(
      'period_reset_at',
      getPeriodResetAt(
        nextType,
        nextType === 'days' ? nextDays : 0,
        Date.now(),
        0,
      ),
    );
  };

  const setPeriodUnit = (nextUnit) => {
    if (!formApiRef.current || (nextUnit !== 'cny' && nextUnit !== 'quota')) {
      return;
    }
    const currentUnit = formApiRef.current.getValue('period_limit_unit') || 'cny';
    const currentValue = String(
      formApiRef.current.getValue('period_limit_value') || '',
    ).trim();
    const converted = convertPeriodLimitUnit(
      currentValue,
      currentUnit,
      nextUnit,
      periodConversion,
      periodCanonicalQuotaRef.current,
    );
    periodCanonicalQuotaRef.current = converted.canonicalQuota;
    formApiRef.current.setValue('period_limit_unit', nextUnit);
    formApiRef.current.setValue('period_limit_value', converted.value);
  };

  const setPeriodLimitValue = (value) => {
    if (!formApiRef.current) return;
    const inputValue = value?.target?.value ?? value ?? '';
    const text = String(inputValue);
    const unit = formApiRef.current.getValue('period_limit_unit') || 'cny';
    if (unit === 'cny') {
      periodCanonicalQuotaRef.current = amountToCanonicalQuota(
        text.trim(),
        periodConversion.displayRate,
        periodConversion.quotaPerUnit,
      );
    } else if (isPositiveIntegerString(text.trim())) {
      periodCanonicalQuotaRef.current = Number(text.trim());
    } else {
      periodCanonicalQuotaRef.current = null;
    }
    formApiRef.current.setValue('period_limit_value', text);
  };

  const getPeriodPreviewResetAt = (values) => {
    if (!values.period_enabled || !values.period_type) return 0;
    if (Number(values.period_reset_at) > 0) {
      return Number(values.period_reset_at);
    }
    return getPeriodResetAt(
      values.period_type,
      values.period_type === 'days' ? Number(values.period_days) : 0,
      Date.now(),
      periodAnchorAtRef.current,
    );
  };

  return (
    <SideSheet
      placement={isEdit ? 'right' : 'left'}
      title={
        <Space>
          {isEdit ? (
            <Tag color='blue' shape='circle'>
              {t('更新')}
            </Tag>
          ) : (
            <Tag color='green' shape='circle'>
              {t('新建')}
            </Tag>
          )}
          <Title heading={4} className='m-0'>
            {isEdit ? t('更新令牌信息') : t('创建新的令牌')}
          </Title>
        </Space>
      }
      bodyStyle={{ padding: '0' }}
      visible={props.visiable}
      width={isMobile ? '100%' : 600}
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button
              theme='solid'
              className='!rounded-lg'
              onClick={() => formApiRef.current?.submitForm()}
              icon={<IconSave />}
              loading={loading}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              className='!rounded-lg'
              type='primary'
              onClick={handleCancel}
              icon={<IconClose />}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      }
      closeIcon={null}
      onCancel={() => handleCancel()}
    >
      <Spin spinning={loading}>
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {({ values }) => (
            <div className='p-2'>
              {/* 基本信息 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconKey size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的基本信息')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Input
                      field='name'
                      label={t('名称')}
                      placeholder={t('请输入名称')}
                      rules={[{ required: true, message: t('请输入名称') }]}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    {groups.length > 0 ? (
                      <Form.Select
                        field='group'
                        label={t('令牌分组')}
                        placeholder={t('令牌分组，默认为用户的分组')}
                        optionList={groups}
                        renderOptionItem={renderGroupOption}
                        filter={(input, option) => {
                          const q = input.toLowerCase();
                          return (
                            option.value?.toLowerCase().includes(q) ||
                            (typeof option.label === 'string' &&
                              option.label.toLowerCase().includes(q))
                          );
                        }}
                        showClear
                        style={{ width: '100%' }}
                      />
                    ) : (
                      <Form.Select
                        placeholder={t('管理员未设置用户可选分组')}
                        disabled
                        label={t('令牌分组')}
                        style={{ width: '100%' }}
                      />
                    )}
                  </Col>
                  <Col
                    span={24}
                    style={{
                      display: values.group === 'auto' ? 'block' : 'none',
                    }}
                  >
                    <Form.Switch
                      field='cross_group_retry'
                      label={t('跨分组重试')}
                      size='default'
                      extraText={t(
                        '开启后，当前分组渠道失败时会按顺序尝试下一个分组的渠道',
                      )}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                    <Form.DatePicker
                      field='expired_time'
                      label={t('过期时间')}
                      type='dateTime'
                      placeholder={t('请选择过期时间')}
                      rules={[
                        { required: true, message: t('请选择过期时间') },
                        {
                          validator: (rule, value) => {
                            // 允许 -1 表示永不过期，也允许空值在必填校验时被拦截
                            if (value === -1 || !value)
                              return Promise.resolve();
                            const time = Date.parse(value);
                            if (isNaN(time)) {
                              return Promise.reject(t('过期时间格式错误！'));
                            }
                            if (time <= Date.now()) {
                              return Promise.reject(
                                t('过期时间不能早于当前时间！'),
                              );
                            }
                            return Promise.resolve();
                          },
                        },
                      ]}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                    <Form.Slot label={t('过期时间快捷设置')}>
                      <Space wrap>
                        <Button
                          theme='light'
                          type='primary'
                          onClick={() => setExpiredTime(0, 0, 0, 0)}
                        >
                          {t('永不过期')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(1, 0, 0, 0)}
                        >
                          {t('一个月')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 1, 0, 0)}
                        >
                          {t('一天')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 0, 1, 0)}
                        >
                          {t('一小时')}
                        </Button>
                      </Space>
                    </Form.Slot>
                  </Col>
                  {!isEdit && (
                    <Col span={24}>
                      <Form.InputNumber
                        field='tokenCount'
                        label={t('新建数量')}
                        min={1}
                        extraText={t('批量创建时会在名称后自动添加随机后缀')}
                        rules={[
                          { required: true, message: t('请输入新建数量') },
                        ]}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  )}
                </Row>
              </Card>

              {/* 额度设置 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <IconCreditCard size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('额度设置')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌可用额度和数量')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.InputNumber
                      field='remain_amount'
                      label={t('金额')}
                      prefix={getCurrencyConfig().symbol}
                      placeholder={t('输入金额')}
                      precision={6}
                      disabled={values.unlimited_quota}
                      min={0}
                      step={0.000001}
                      onChange={(val) => {
                        const amount = val === '' || val == null ? 0 : val;
                        formApiRef.current?.setValue('remain_amount', amount);
                        formApiRef.current?.setValue(
                          'remain_quota',
                          displayAmountToQuota(amount),
                        );
                      }}
                      style={{ width: '100%' }}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    <div
                      className='text-xs cursor-pointer mt-1'
                      style={{ color: 'var(--semi-color-text-2)' }}
                      onClick={() => setShowQuotaInput((v) => !v)}
                    >
                      {showQuotaInput
                        ? `▾ ${t('收起原生额度输入')}`
                        : `▸ ${t('使用原生额度输入')}`}
                    </div>
                    <div style={{ display: showQuotaInput ? 'block' : 'none' }} className='mt-2'>
                      <Form.InputNumber
                        field='remain_quota'
                        label={t('额度')}
                        placeholder={t('输入额度')}
                        disabled={values.unlimited_quota}
                        min={0}
                        step={500000}
                        rules={
                          values.unlimited_quota
                            ? []
                            : [{ required: true, message: t('请输入额度') }]
                        }
                        onChange={(val) => {
                          const quota = val === '' || val == null ? 0 : val;
                          formApiRef.current?.setValue('remain_quota', quota);
                          formApiRef.current?.setValue(
                            'remain_amount',
                            Number(quotaToDisplayAmount(quota).toFixed(6)),
                          );
                        }}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </div>
                  </Col>
                  <Col span={24}>
                    <Form.Switch
                      field='unlimited_quota'
                      label={t('无限额度')}
                      size='default'
                      extraText={t(
                        '令牌的额度仅用于限制令牌本身的最大额度使用量，实际的使用受到账户的剩余额度限制',
                      )}
                    />
                  </Col>
                </Row>
              </Card>

              {/* 周期限额 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='orange' className='mr-2 shadow-md'>
                    <IconCreditCard size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('周期限额')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置每个周期的令牌额度上限')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Switch
                      field='period_enabled'
                      label={t('启用周期限额')}
                      size='default'
                      onChange={setPeriodEnabled}
                      extraText={t('周期限额独立统计，不受令牌无限额度开关影响')}
                    />
                  </Col>
                  {values.period_enabled && (
                    <>
                      <Col span={24}>
                        <Form.Select
                          field='period_type'
                          label={t('周期类型')}
                          optionList={[
                            { value: 'days', label: t('每 N 天') },
                            { value: 'week', label: t('每周') },
                            { value: 'month', label: t('每月') },
                          ]}
                          onChange={setPeriodType}
                          getPopupContainer={() => document.body}
                          style={{ width: '100%' }}
                        />
                      </Col>
                      {values.period_type === 'days' && (
                        <Col span={24}>
                          <Form.InputNumber
                            field='period_days'
                            label={t('周期天数')}
                            min={1}
                            max={TOKEN_PERIOD_MAX_DAYS}
                            step={1}
                            precision={0}
                            rules={[
                              {
                                validator: (_rule, value) => {
                                  const days = Number(value);
                                  if (
                                    Number.isInteger(days) &&
                                    days >= 1 &&
                                    days <= TOKEN_PERIOD_MAX_DAYS
                                  ) {
                                    return Promise.resolve();
                                  }
                                  return Promise.reject(
                                    t('周期天数必须在 1 到 3650 之间'),
                                  );
                                },
                              },
                            ]}
                            onChange={(value) => {
                              const days = Number(value) || 0;
                              periodAnchorAtRef.current = 0;
                              formApiRef.current?.setValue('period_days', days);
                              formApiRef.current?.setValue(
                                'period_reset_at',
                                getPeriodResetAt('days', days),
                              );
                            }}
                            style={{ width: '100%' }}
                          />
                        </Col>
                      )}
                      <Col span={24}>
                        <Form.Select
                          field='period_limit_unit'
                          label={t('周期限额单位')}
                          optionList={[
                            { value: 'cny', label: periodAmountUnitLabel },
                            { value: 'quota', label: t('原生额度') },
                          ]}
                          onChange={setPeriodUnit}
                          getPopupContainer={() => document.body}
                          style={{ width: '100%' }}
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Input
                          field='period_limit_value'
                          label={t('周期限额值')}
                          prefix={
                            values.period_limit_unit === 'cny'
                              ? periodConversion.symbol || undefined
                              : undefined
                          }
                          extraText={t('填 0 即关闭周期限额')}
                          inputMode={
                            values.period_limit_unit === 'quota'
                              ? 'numeric'
                              : 'decimal'
                          }
                          placeholder={
                            values.period_limit_unit === 'cny'
                              ? periodAmountPlaceholder
                              : String(Math.max(1, Math.trunc(periodConversion.quotaPerUnit)))
                          }
                          onChange={setPeriodLimitValue}
                          rules={[
                            {
                              validator: (_rule, value) => {
                                const result = validatePeriodForm(
                                  { ...values, period_limit_value: value },
                                  periodConversion,
                                );
                                return result.valid
                                  ? Promise.resolve()
                                  : Promise.reject(
                                      t(getPeriodValidationMessage(result.errors)),
                                    );
                              },
                            },
                          ]}
                          style={{ width: '100%' }}
                          showClear
                        />
                      </Col>
                      <Col span={24}>
                        <Form.Slot label={t('下次重置时间')}>
                          <Tag color='orange' shape='circle'>
                            {formatPeriodResetAt(
                              getPeriodPreviewResetAt(values),
                              i18n.resolvedLanguage || i18n.language,
                            )}
                          </Tag>
                        </Form.Slot>
                      </Col>
                    </>
                  )}
                </Row>
              </Card>

              {/* 访问限制 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar
                    size='small'
                    color='purple'
                    className='mr-2 shadow-md'
                  >
                    <IconLink size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('访问限制')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的访问限制')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Select
                      field='model_limits'
                      label={t('模型限制列表')}
                      placeholder={t(
                        '请选择该令牌支持的模型，留空支持所有模型',
                      )}
                      multiple
                      optionList={models}
                      extraText={t('非必要，不建议启用模型限制')}
                      filter={selectFilter}
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.TextArea
                      field='allow_ips'
                      label={t('IP白名单（支持CIDR表达式）')}
                      placeholder={t('允许的IP，一行一个，不填写则不限制')}
                      autosize
                      rows={1}
                      extraText={t(
                        '请勿过度信任此功能，IP可能被伪造，请配合nginx和cdn等网关使用',
                      )}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>
            </div>
          )}
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditTokenModal;

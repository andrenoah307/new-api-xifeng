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
import { Avatar, Typography, Table, Tag, Tooltip } from '@douyinfe/semi-ui';
import { IconCoinMoneyStroked, IconHelpCircle } from '@douyinfe/semi-icons';
import {
  calculateModelPrice,
  getModelPriceItems,
  stringToColor,
} from '../../../../../helpers';

const { Text } = Typography;

const ModelPricingTable = ({
  modelData,
  groupRatio,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  usableGroup,
  autoGroups = [],
  t,
}) => {
  const modelEnableGroups = Array.isArray(modelData?.enable_groups)
    ? modelData.enable_groups
    : [];
  const autoChain = autoGroups.filter((g) => modelEnableGroups.includes(g));
  const renderGroupPriceTable = () => {
    // 仅展示模型可用的分组：模型 enable_groups 与用户可用分组的交集

    const availableGroups = Object.keys(usableGroup || {})
      .filter((g) => g !== '')
      .filter((g) => g !== 'auto')
      .filter((g) => modelEnableGroups.includes(g));

    // 准备表格数据
    const tableData = availableGroups.map((group) => {
      const configuredRatio = groupRatio?.[group];
      const groupRatioValue = Number.isFinite(configuredRatio)
        ? configuredRatio
        : 1;
      const priceData = modelData
        ? calculateModelPrice({
            record: modelData,
            selectedGroup: group,
            groupRatio: { ...groupRatio, [group]: groupRatioValue },
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          })
        : { inputPrice: '-', outputPrice: '-', price: '-' };

      return {
        key: group,
        group: group,
        ratio: groupRatioValue,
        billingType:
          modelData?.billing_mode === 'tiered_expr'
            ? t('动态计费')
            : modelData?.quota_type === 0
              ? t('按量计费')
              : modelData?.quota_type === 1
                ? t('按次计费')
                : '-',
        priceItems: getModelPriceItems(priceData, t, siteDisplayType),
      };
    });

    // 定义表格列
    const columns = [
      {
        title: t('分组'),
        dataIndex: 'group',
        width: 110,
        render: (text) => (
          <Tag color={stringToColor(text)} size='small' shape='circle'>
            {text}
          </Tag>
        ),
      },
    ];

    const discountColumn = {
      title: (
        <span className='inline-flex items-center gap-1'>
          {t('Discount Rate')}
          <Tooltip
            content={t(
              'Not a live exchange rate. Calculated using a fixed reference rate of CNY 7 : USD 1.',
            )}
          >
            <button
              type='button'
              aria-label={t('Discount Rate')}
              className='inline-flex w-5 h-5 items-center justify-center rounded border-0 bg-transparent p-0 text-inherit'
            >
              <IconHelpCircle aria-hidden='true' />
            </button>
          </Tooltip>
        </span>
      ),
      dataIndex: 'ratio',
      width: 155,
      render: (ratio) => {
        const discount = Math.round((1 - ratio / 7) * 100);
        const label = t('{{percent}}% off at USD/CNY 7', { percent: discount });
        let color = 'grey';
        if (discount > 0) {
          color = 'green';
        } else if (discount < 0) {
          color = 'orange';
        }
        return (
          <span
            title={t(
              'Discount = (1 - group ratio / 7) × 100%, rounded to a whole percent. Uses a fixed reference rate of USD 1 = CNY 7, independent of site exchange rates and recharge discounts.',
            )}
          >
            <Tag color={color} size='small' shape='circle'>
              {label}
            </Tag>
          </span>
        );
      },
    };

    columns.push({
      title: t('计费类型'),
      dataIndex: 'billingType',
      width: 125,
      render: (text) => {
        let color = 'white';
        if (text === t('按量计费')) color = 'violet';
        else if (text === t('按次计费')) color = 'teal';
        else if (text === t('动态计费')) color = 'amber';
        return (
          <Tag color={color} size='small' shape='circle'>
            {text || '-'}
          </Tag>
        );
      },
    });

    columns.push({
      title: siteDisplayType === 'TOKENS' ? t('计费摘要') : t('价格摘要'),
      dataIndex: 'priceItems',
      width: 230,
      render: (items) => {
        if (items.length === 1 && items[0].isDynamic) {
          return (
            <Text type='tertiary' size='small'>
              {t('见下方动态计费详情')}
            </Text>
          );
        }
        return (
          <div className='space-y-1'>
            {items.map((item) => (
              <div key={item.key}>
                <div className='font-semibold text-orange-600'>
                  {item.label} {item.value}
                </div>
                <div className='text-xs text-gray-500'>{item.suffix}</div>
              </div>
            ))}
          </div>
        );
      },
    });

    columns.push(discountColumn);

    return (
      <Table
        dataSource={tableData}
        columns={columns}
        pagination={false}
        size='small'
        scroll={{ x: 620 }}
        bordered={false}
        className='!rounded-lg'
      />
    );
  };

  return (
    <div>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='orange' className='mr-2 shadow-md'>
          <IconCoinMoneyStroked size={16} />
        </Avatar>
        <div>
          <Text className='text-lg font-medium'>{t('分组价格')}</Text>
          <div className='text-xs text-gray-600'>
            {t('不同用户分组的价格信息')}
          </div>
        </div>
      </div>
      {autoChain.length > 0 && (
        <div className='flex flex-wrap items-center gap-1 mb-4'>
          <span className='text-sm text-gray-600'>{t('auto分组调用链路')}</span>
          <span className='text-sm'>→</span>
          {autoChain.map((g, idx) => (
            <React.Fragment key={g}>
              <Tag color={stringToColor(g)} size='small' shape='circle'>
                {g}
                {t('分组')}
              </Tag>
              {idx < autoChain.length - 1 && <span className='text-sm'>→</span>}
            </React.Fragment>
          ))}
        </div>
      )}
      {renderGroupPriceTable()}
    </div>
  );
};

export default ModelPricingTable;

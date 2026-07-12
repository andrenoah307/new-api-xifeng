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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Table,
  Button,
  Tag,
  Space,
  Popconfirm,
  Empty,
  Input,
  Switch,
  Card,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { IconSearch, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import EditDiscountCode from './EditDiscountCode';

const ITEMS_PER_PAGE = 10;

const DiscountCode = () => {
  const { t } = useTranslation();

  const [discountCodes, setDiscountCodes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');

  // Modal state
  const [showEditModal, setShowEditModal] = useState(false);
  const [editingRecord, setEditingRecord] = useState(null);

  const loadData = async (page = 1, size = pageSize) => {
    setLoading(true);
    try {
      const res = await API.get(
        `/api/discount_code/?p=${page}&page_size=${size}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        setDiscountCodes(data.items || []);
        setActivePage(data.page <= 0 ? 1 : data.page);
        setTotal(data.total || 0);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  const searchData = async (keyword) => {
    if (!keyword) {
      await loadData(1, pageSize);
      return;
    }
    setLoading(true);
    try {
      const res = await API.get(
        `/api/discount_code/search?keyword=${encodeURIComponent(keyword)}&p=1&page_size=${pageSize}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        setDiscountCodes(data.items || []);
        setActivePage(data.page || 1);
        setTotal(data.total || 0);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  const handleDelete = async (id) => {
    setLoading(true);
    try {
      const res = await API.delete(`/api/discount_code/${id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('操作成功完成！'));
        await refresh();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  const handleToggleStatus = async (record) => {
    setLoading(true);
    try {
      const newStatus = record.status === 1 ? 2 : 1;
      const res = await API.put('/api/discount_code/?status_only=true', {
        id: record.id,
        status: newStatus,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('操作成功完成！'));
        const updated = discountCodes.map((item) =>
          item.id === record.id ? { ...item, status: newStatus } : item,
        );
        setDiscountCodes(updated);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  const handleCleanup = async (record) => {
    setLoading(true);
    try {
      const res = await API.post(`/api/discount_code/${record.id}/cleanup`);
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('已清理 {{count}} 笔未付款订单', { count: data || 0 }));
        await refresh();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  const refresh = async () => {
    if (searchKeyword) {
      await searchData(searchKeyword);
    } else {
      await loadData(activePage, pageSize);
    }
  };

  const handlePageChange = (page) => {
    setActivePage(page);
    if (searchKeyword) {
      searchData(searchKeyword);
    } else {
      loadData(page, pageSize);
    }
  };

  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
    if (searchKeyword) {
      searchData(searchKeyword);
    } else {
      loadData(1, size);
    }
  };

  useEffect(() => {
    loadData(1, pageSize);
  }, []);

  const renderUsesCount = (value) => {
    return value === 0 ? (
      <Tag color='blue' shape='circle'>
        {t('无限')}
      </Tag>
    ) : (
      value
    );
  };

  const columns = useMemo(
    () => [
      {
        title: 'ID',
        dataIndex: 'id',
        width: 60,
      },
      {
        title: t('折扣码'),
        dataIndex: 'code',
        width: 200,
        render: (text) => (
          <Typography.Text copyable style={{ fontFamily: 'monospace' }}>
            {text}
          </Typography.Text>
        ),
      },
      {
        title: t('名称'),
        dataIndex: 'name',
        width: 150,
      },
      {
        title: t('折扣率'),
        dataIndex: 'discount_rate',
        width: 120,
        render: (value) => (
          <Tag color='green' shape='circle'>
            {value}% = {(value / 10).toFixed(1)}
            {t('折')}
          </Tag>
        ),
      },
      {
        title: t('开始时间'),
        dataIndex: 'start_time',
        width: 170,
        render: (text) =>
          text && text > 0 ? timestamp2string(text) : t('无'),
      },
      {
        title: t('结束时间'),
        dataIndex: 'end_time',
        width: 170,
        render: (text) =>
          text && text > 0 ? timestamp2string(text) : t('无'),
      },
      {
        title: t('单用户使用次数'),
        dataIndex: 'max_uses_per_user',
        width: 130,
        render: renderUsesCount,
      },
      {
        title: t('总使用次数'),
        dataIndex: 'max_uses_total',
        width: 110,
        render: renderUsesCount,
      },
      {
        title: t('已使用次数'),
        dataIndex: 'used_count',
        width: 100,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: 80,
        render: (status) =>
          status === 1 ? (
            <Tag color='green' shape='circle'>
              {t('启用')}
            </Tag>
          ) : (
            <Tag color='red' shape='circle'>
              {t('禁用')}
            </Tag>
          ),
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        fixed: 'right',
        width: 280,
        render: (_, record) => (
          <Space>
            <Button
              type='tertiary'
              size='small'
              onClick={() => {
                setEditingRecord(record);
                setShowEditModal(true);
              }}
            >
              {t('编辑')}
            </Button>
            <Switch
              size='small'
              checked={record.status === 1}
              onChange={() => handleToggleStatus(record)}
            />
            <Popconfirm
              title={t('确认清理该折扣码关联的所有未付款订单？')}
              onConfirm={() => handleCleanup(record)}
              position='left'
            >
              <Button type='tertiary' size='small'>
                {t('清理未付款')}
              </Button>
            </Popconfirm>
            <Popconfirm
              title={t('确定是否要删除此折扣码？')}
              onConfirm={() => handleDelete(record.id)}
              position='left'
            >
              <Button type='danger' size='small'>
                {t('删除')}
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [t, discountCodes],
  );

  return (
    <div className='mt-[60px] px-2'>
      <Card className='!rounded-xl'>
        <div className='flex flex-wrap justify-between items-center mb-4 gap-2'>
          <Space>
            <Input
              prefix={<IconSearch />}
              placeholder={t('搜索折扣码或名称')}
              value={searchKeyword}
              onChange={(val) => setSearchKeyword(val)}
              onEnterPress={() => searchData(searchKeyword)}
              showClear
              onClear={() => {
                setSearchKeyword('');
                loadData(1, pageSize);
              }}
              style={{ width: 240 }}
            />
            <Button onClick={() => searchData(searchKeyword)}>
              {t('搜索')}
            </Button>
          </Space>
          <Button
            type='primary'
            theme='solid'
            icon={<IconPlus />}
            onClick={() => {
              setEditingRecord(null);
              setShowEditModal(true);
            }}
          >
            {t('新建')}
          </Button>
        </div>

        <Table
          columns={columns}
          dataSource={discountCodes}
          rowKey='id'
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: activePage,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50, 100],
            onPageSizeChange: handlePageSizeChange,
            onPageChange: handlePageChange,
          }}
          loading={loading}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark
                  style={{ width: 150, height: 150 }}
                />
              }
              description={t('搜索无结果')}
              style={{ padding: 30 }}
            />
          }
          className='rounded-xl overflow-hidden'
          size='middle'
        />
      </Card>

      {showEditModal && (
        <EditDiscountCode
          visible={showEditModal}
          record={editingRecord}
          onClose={() => {
            setShowEditModal(false);
            setEditingRecord(null);
          }}
          onSuccess={() => {
            setShowEditModal(false);
            setEditingRecord(null);
            refresh();
          }}
        />
      )}
    </div>
  );
};

export default DiscountCode;

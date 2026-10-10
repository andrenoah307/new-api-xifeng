import React, { useEffect, useState } from "react";
import { Button, Select, Typography } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { API } from "../../../helpers";

export default function MonitoringExclusionSelect({ kind, value, onChange, id }) {
  const { t } = useTranslation();
  const [keyword, setKeyword] = useState("");
  const [rows, setRows] = useState([]);
  const [names, setNames] = useState({});
  const [loading, setLoading] = useState(false);
  const [searchError, setSearchError] = useState(false);
  const [savedError, setSavedError] = useState(false);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setSavedError(false);
    API.get("/api/monitoring/admin/candidates", {
      params: { kind, selected: true },
      signal: controller.signal,
      disableDuplicate: true,
      skipErrorHandler: true,
    })
      .then(({ data }) => {
        if (!data.success) throw new Error();
        if (active)
          setNames((previous) => ({
            ...previous,
            ...Object.fromEntries(data.data.map((row) => [row.id, row.name])),
          }));
      })
      .catch(() => {
        if (active) setSavedError(true);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [kind, retry]);

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setRows([]);
    setSearchError(false);
    setLoading(Boolean(keyword.trim()));
    const timer = setTimeout(async () => {
      if (!keyword.trim()) return;
      try {
        const { data } = await API.get("/api/monitoring/admin/candidates", {
          params: { kind, keyword: keyword.trim() },
          signal: controller.signal,
          disableDuplicate: true,
          skipErrorHandler: true,
        });
        if (!data.success) throw new Error();
        if (active) {
          setRows(data.data);
          setNames((previous) => ({
            ...previous,
            ...Object.fromEntries(data.data.map((row) => [row.id, row.name])),
          }));
        }
      } catch {
        if (active) setSearchError(true);
      } finally {
        if (active) setLoading(false);
      }
    }, 350);
    return () => {
      active = false;
      clearTimeout(timer);
      controller.abort();
    };
  }, [kind, keyword, retry]);

  const options = new Map(
    rows.map((row) => [
      String(row.id),
      { value: String(row.id), label: `${row.name} (#${row.id})` },
    ]),
  );
  for (const selected of value) {
    options.set(selected, {
      value: selected,
      label: names[selected] ? `${names[selected]} (#${selected})` : `#${selected}`,
    });
  }
  return (
    <>
      <Select
        id={id}
        aria-labelledby={`${id}-label`}
        inputProps={{ id: `${id}-input`, 'aria-label': t('Search by name'), maxLength: 50 }}
        multiple
        remote
        filter
        loading={loading}
        value={value}
        onChange={onChange}
        onSearch={(text) => setKeyword(text.slice(0, 50))}
        placeholder={t("Search by name")}
        optionList={[...options.values()]}
        emptyContent={t('暂无数据')}
        innerBottomSlot={searchError && <div role="alert">
          <Typography.Text type="danger">{t('加载失败')}</Typography.Text>
          <Button onClick={() => setRetry((count) => count + 1)}>{t('重试')}</Button>
        </div>}
        style={{ width: "100%" }}
        showClear
      />
      {savedError && (
        <div role="alert">
          <Typography.Text type="danger">{t('加载失败')}</Typography.Text>
          <Button onClick={() => setRetry((count) => count + 1)}>{t('重试')}</Button>
        </div>
      )}
    </>
  );
}

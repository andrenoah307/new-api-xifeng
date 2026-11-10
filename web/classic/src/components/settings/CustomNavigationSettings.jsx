import React, { useContext, useState } from "react";
import { Button, Checkbox, Input, Select } from "@douyinfe/semi-ui";
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { API, showError, showSuccess } from "../../helpers";
import { parseCustomNavigationLinks } from "../../helpers/custom-navigation";
import { StatusContext } from "../../context/Status";

export default function CustomNavigationSettings({ value }) {
  const { t } = useTranslation();
  const [, dispatch] = useContext(StatusContext);
  const [links, setLinks] = useState(() => parseCustomNavigationLinks(value));
  const [saving, setSaving] = useState(false);
  const change = (index, patch) =>
    setLinks((previous) => previous.map((link, i) => (i === index ? { ...link, ...patch } : link)));
  const move = (index, direction) =>
    setLinks((previous) => {
      const next = [...previous];
      const target = index + direction;
      if (target < 0 || target >= next.length) return previous;
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
  const save = async () => {
    if (saving) return;
    setSaving(true);
    try {
      const res = await API.put("/api/option/", {
        key: "CustomNavigationLinks",
        value: JSON.stringify(links),
      });
      if (!res.data?.success) throw new Error(res.data?.message || t("保存失败"));
      const status = await API.get("/api/status");
      if (status.data?.success) {
        dispatch({ type: "set", payload: status.data.data });
        localStorage.setItem("status", JSON.stringify(status.data.data));
      }
      showSuccess(t("保存成功"));
    } catch (error) {
      showError(error.message || t("请求失败"));
    } finally {
      setSaving(false);
    }
  };
  return (
    <section className="mt-6 border-t pt-6">
      <h3 className="text-base font-semibold mb-4">{t("Custom navigation links")}</h3>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
      >
        <fieldset disabled={saving} className="min-w-0 space-y-4">
          {links.map((link, index) => (
            <div key={link.id} className="space-y-3 border-b pb-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="min-w-0">
                  {t("名称")}
                  <Input
                    required
                    maxLength={80}
                    value={link.title}
                    onChange={(title) => change(index, { title })}
                  />
                </label>
                <label className="min-w-0">
                  URL
                  <Input
                    type="url"
                    required
                    maxLength={2048}
                    value={link.url}
                    placeholder="https://example.com"
                    onChange={(url) => change(index, { url })}
                  />
                </label>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <Select
                  aria-label={t("Open mode")}
                  value={link.mode}
                  onChange={(mode) => change(index, { mode })}
                  optionList={[
                    { value: "external", label: t("Open in new tab") },
                    { value: "iframe", label: t("Embed in page") },
                  ]}
                />
                <Checkbox
                  checked={link.header}
                  onChange={(event) => change(index, { header: event.target.checked })}
                >
                  {t("Top navigation")}
                </Checkbox>
                <span id={`${link.id}-style-label`} className="sr-only">{t("Custom header link style")}</span>
                <Select
                  aria-labelledby={`${link.id}-style-label`}
                  value={link.headerStyle === "packy" ? "packy" : "default"}
                  disabled={saving}
                  onChange={(headerStyle) => change(index, { headerStyle })}
                  optionList={[
                    { value: "default", label: t("Default style") },
                    { value: "packy", label: t("Packy style") },
                  ]}
                />
                <Checkbox
                  checked={link.sidebar}
                  onChange={(event) => change(index, { sidebar: event.target.checked })}
                >
                  {t("User sidebar")}
                </Checkbox>
                <Checkbox checked={link.home === true} onChange={(event) => change(index, { home: event.target.checked })}>
                  {t("Homepage (after Docs)")}
                </Checkbox>
                <div className="ml-auto flex gap-1">
                  <Button
                    htmlType="button"
                    title={t("Move up")}
                    aria-label={t("Move up")}
                    icon={<ArrowUp size={16} />}
                    disabled={index === 0 || saving}
                    onClick={() => move(index, -1)}
                  />
                  <Button
                    htmlType="button"
                    title={t("Move down")}
                    aria-label={t("Move down")}
                    icon={<ArrowDown size={16} />}
                    disabled={index === links.length - 1 || saving}
                    onClick={() => move(index, 1)}
                  />
                  <Button
                    htmlType="button"
                    title={t("删除")}
                    aria-label={t("删除")}
                    icon={<Trash2 size={16} />}
                    onClick={() =>
                      setLinks((previous) => previous.filter((item) => item.id !== link.id))
                    }
                  />
                </div>
              </div>
            </div>
          ))}
          <div className="flex gap-2">
            <Button
              htmlType="button"
              icon={<Plus size={16} />}
              disabled={saving || links.length >= 30}
              onClick={() =>
                setLinks((previous) => [
                  ...previous,
                  {
                    id: crypto.randomUUID(),
                    title: "",
                    url: "",
                    mode: "external",
                    header: true,
                    sidebar: true,
                  },
                ])
              }
            >
              {t("添加")}
            </Button>
            <Button htmlType="submit" theme="solid" loading={saving}>
              {t("保存设置")}
            </Button>
          </div>
        </fieldset>
      </form>
    </section>
  );
}

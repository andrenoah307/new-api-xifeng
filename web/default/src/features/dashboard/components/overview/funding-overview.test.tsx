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
import { test } from "node:test";
import assert from "node:assert/strict";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { renderToStaticMarkup } from "react-dom/server";
import { useAuthStore } from "@/stores/auth-store";
import { FundingOverview } from "./funding-overview";

test("funding amounts and percentages render for interface language codes", async () => {
  const initialState = useAuthStore.getInitialState();
  const oldUser = initialState.auth.user;
  initialState.auth.user = { id: 1, username: "root", role: 100 };
  try {
    for (const [language, locale] of [
      ["zhCN", "zh-CN"],
      ["zhTW", "zh-TW"],
      ["en", "en"],
      ["fr", "fr"],
      ["ja", "ja"],
      ["ru", "ru"],
      ["vi", "vi"],
      ["invalid_locale!", undefined],
    ] as const) {
      const i18n = createInstance();
      await i18n.init({
        lng: language,
        fallbackLng: "en",
        resources: { en: { translation: {} } },
      });
      const client = new QueryClient();
      const end = Math.floor(Date.now() / 3600000) * 3600;
      client.setQueryData(["root-funding-overview", 1, end - 86400, end], {
        consumption: "1234.56",
        online_topups: "100",
        manual_credits: "100",
        total_funding: "200",
        consumption_ratio: 0.25,
        calculated_at: end,
      });
      try {
        const markup = renderToStaticMarkup(
          <I18nextProvider i18n={i18n}>
            <QueryClientProvider client={client}>
              <FundingOverview />
            </QueryClientProvider>
          </I18nextProvider>,
        );
        assert.ok(
          markup.includes(
            new Intl.NumberFormat(locale, {
              style: "currency",
              currency: "CNY",
              maximumFractionDigits: 2,
            }).format(1234.56),
          ),
          language,
        );
        assert.ok(
          markup.includes(
            new Intl.NumberFormat(locale, {
              style: "percent",
              maximumFractionDigits: 2,
            }).format(0.25),
          ),
          language,
        );
      } finally {
        client.clear();
      }
    }
  } finally {
    initialState.auth.user = oldUser;
  }
});

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

function normalizeCacheToken(value) {
  const token = Number(value);
  if (
    !Number.isFinite(token) ||
    token <= 0 ||
    token > Number.MAX_SAFE_INTEGER
  ) {
    return 0;
  }
  return token;
}

export function getPromptCacheSummary(other) {
  if (!other || typeof other !== 'object') {
    return null;
  }

  const cacheReadTokens = normalizeCacheToken(other.cache_tokens);
  const cacheCreationTokens = normalizeCacheToken(other.cache_creation_tokens);
  const cacheCreationTokens5m = normalizeCacheToken(
    other.cache_creation_tokens_5m,
  );
  const cacheCreationTokens1h = normalizeCacheToken(
    other.cache_creation_tokens_1h,
  );
  const hasSplitCacheCreation =
    cacheCreationTokens5m > 0 || cacheCreationTokens1h > 0;
  const genericCreationTokens = hasSplitCacheCreation
    ? Math.max(
        0,
        cacheCreationTokens - cacheCreationTokens5m - cacheCreationTokens1h,
      )
    : cacheCreationTokens;
  const cacheWriteTokens =
    genericCreationTokens + cacheCreationTokens5m + cacheCreationTokens1h;

  if (cacheReadTokens <= 0 && cacheWriteTokens <= 0) {
    return null;
  }

  return {
    cacheReadTokens,
    cacheWriteTokens,
  };
}

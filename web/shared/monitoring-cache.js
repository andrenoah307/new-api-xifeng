// Copyright (C) 2023-2026 QuantumNous

export function monitoringCacheRate(tokenRate, requests, hits) {
  const token =
    Number.isFinite(tokenRate) && tokenRate >= 0
      ? Math.min(100, tokenRate)
      : tokenRate;
  if (
    !Number.isSafeInteger(requests) ||
    requests <= 0 ||
    !Number.isSafeInteger(hits) ||
    hits < 0 ||
    hits > requests
  ) {
    return token;
  }
  const requestRate = (hits / requests) * 100;
  return Number.isFinite(token) && token >= 0
    ? Math.max(token, requestRate)
    : requestRate;
}

export function withMonitoringCacheRates(sample) {
  const result = {
    ...sample,
    cache_hit_rate: monitoringCacheRate(
      sample.cache_hit_rate,
      sample.cache_request_count,
      sample.cache_hit_request_count,
    ),
  };
  if ("cache_hit_rate_1h" in sample) {
    result.cache_hit_rate_1h = monitoringCacheRate(
      sample.cache_hit_rate_1h,
      sample.cache_request_count_1h,
      sample.cache_hit_request_count_1h,
    );
  }
  return result;
}

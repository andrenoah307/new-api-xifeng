// Copyright (C) 2023-2026 QuantumNous

export interface MonitoringCacheSample {
  cache_hit_rate: number | null;
  cache_request_count?: number | null;
  cache_hit_request_count?: number | null;
  cache_hit_rate_1h?: number | null;
  cache_request_count_1h?: number | null;
  cache_hit_request_count_1h?: number | null;
}

export function monitoringCacheRate(
  tokenRate: number | null | undefined,
  requests?: number | null,
  hits?: number | null,
): number | null | undefined;

export function withMonitoringCacheRates<T extends MonitoringCacheSample>(
  sample: T,
): T;

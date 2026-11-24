import { describe, it, expect } from "bun:test";
import {
  monitoringCacheRate,
  withMonitoringCacheRates,
} from "./monitoring-cache.js";

describe("monitoring cache display", () => {
  it("chooses the higher valid rate and retains zero and unavailable data", () => {
    expect(monitoringCacheRate(80, 10, 9)).toBe(90);
    expect(monitoringCacheRate(98, 10, 9)).toBe(98);
    expect(monitoringCacheRate(0, 10, 0)).toBe(0);
    expect(monitoringCacheRate(-1, 10, 8)).toBe(80);
    expect(monitoringCacheRate(null, null, null)).toBeNull();
    expect(monitoringCacheRate(-1)).toBe(-1);
    for (const [requests, hits] of [
      [0, 0],
      [10, null],
      [10, 11],
      [10, -1],
      [NaN, 1],
      [10, 1.5],
    ]) {
      expect(monitoringCacheRate(86, requests, hits)).toBe(86);
    }
  });

  it("keeps hourly and configured windows independent without mutating API data", () => {
    const sample = {
      cache_hit_rate: 30,
      cache_request_count: 10,
      cache_hit_request_count: 9,
      cache_hit_rate_1h: 70,
      cache_request_count_1h: 20,
      cache_hit_request_count_1h: 10,
    };
    expect(withMonitoringCacheRates(sample)).toEqual({
      ...sample,
      cache_hit_rate: 90,
    });
    expect(sample.cache_hit_rate).toBe(30);
    expect(withMonitoringCacheRates({ cache_hit_rate: 40 })).toEqual({
      cache_hit_rate: 40,
    });
  });
});

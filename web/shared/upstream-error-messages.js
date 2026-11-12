export const DEFAULT_UPSTREAM_ERROR_MESSAGES = {
  client: "请求参数有误，请检查请求内容",
  server: "上游服务暂不可用，请稍后重试",
};

export function parseUpstreamErrorMessages(raw) {
  const result = { ...DEFAULT_UPSTREAM_ERROR_MESSAGES };
  try {
    const parsed = JSON.parse(raw || "{}");
    for (const key of Object.keys(result)) {
      if (typeof parsed?.[key] === "string" && parsed[key].trim()) {
        result[key] = parsed[key];
      }
    }
  } catch {
    return result;
  }
  return result;
}

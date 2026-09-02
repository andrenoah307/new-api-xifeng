/**
 * 排除分组：命中的用户分组不得通过本渠道选路。
 * 与后端 dto.ChannelSettings.ExcludedUserGroups 同形，去空去重且保留管理员的选择顺序。
 */
export const normalizeExcludedUserGroups = (value) => {
  if (!Array.isArray(value)) return [];
  const seen = new Set();
  const result = [];
  for (const group of value) {
    if (typeof group !== 'string') continue;
    const trimmed = group.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    result.push(trimmed);
  }
  return result;
};

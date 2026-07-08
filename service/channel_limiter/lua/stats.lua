-- 批量读取渠道限流实时统计（只读，不预占）
-- KEYS: [rpmKey1, concKey1, rpmKey2, concKey2, ...]
-- ARGV[1]: 滑动窗口秒数
-- 返回: [rpmCount1, concCount1, rpmCount2, concCount2, ...]

local window = tonumber(ARGV[1])
local n = #KEYS / 2
local results = {}

for i = 1, n do
    local rpmKey = KEYS[i * 2 - 1]
    local concKey = KEYS[i * 2]

    local now = redis.call('TIME')
    local nowMs = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
    redis.call('ZREMRANGEBYSCORE', rpmKey, 0, nowMs - window * 1000)
    local rpmCount = redis.call('ZCARD', rpmKey)
    results[i * 2 - 1] = rpmCount

    local concVal = tonumber(redis.call('GET', concKey)) or 0
    results[i * 2] = concVal
end

return results

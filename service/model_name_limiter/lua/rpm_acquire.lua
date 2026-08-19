-- Atomic model-name RPM sliding-window acquire.
-- KEYS: one to four buckets in caller-defined priority order.
-- ARGV[1]: window length in seconds
-- ARGV[2..#KEYS+1]: limits for the corresponding keys
-- ARGV[#ARGV]: unique member token

local window = tonumber(ARGV[1])
local token = ARGV[#ARGV]
local now = redis.call('TIME')
local nowMs = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)

-- Clean and check every bucket before writing any bucket.
for i = 1, #KEYS do
    local limit = tonumber(ARGV[i + 1])
    local key = KEYS[i]
    redis.call('ZREMRANGEBYSCORE', key, 0, nowMs - window * 1000)
    local current = redis.call('ZCARD', key)
    if limit > 0 and current >= limit then
        return { '0', i, limit, current }
    end
end

for i = 1, #KEYS do
    local key = KEYS[i]
    -- Include the key index so even a malformed duplicate-key request cannot
    -- collapse two writes into one ZSET member.
    redis.call('ZADD', key, nowMs, token .. ':' .. i)
    redis.call('PEXPIRE', key, (window + 5) * 1000)
end

return { '1' }

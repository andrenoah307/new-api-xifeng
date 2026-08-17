-- Read the current members in each RPM bucket.
-- KEYS: any number of buckets to inspect in one invocation.
-- ARGV[1]: sliding-window length in seconds

local window = tonumber(ARGV[1])
local now = redis.call('TIME')
local nowMs = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
local lower = nowMs - window * 1000
local counts = {}

for i = 1, #KEYS do
    counts[i] = redis.call('ZCOUNT', KEYS[i], '(' .. lower, '+inf')
end

return counts

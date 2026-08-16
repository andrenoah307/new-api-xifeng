-- Read the token bucket state without changing it.
-- KEYS[1]: bucket key
-- ARGV[1]: requested token count (accepted for option parity with Allow)
-- ARGV[2]: token generation rate per second
-- ARGV[3]: bucket capacity

local key = KEYS[1]
local requested = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])

local now = redis.call('TIME')
local nowInSeconds = tonumber(now[1])
local bucket = redis.call('HMGET', key, 'tokens', 'last_time')

if not bucket[1] or not bucket[2] then
    return { 'missing' }
end

local tokens = tonumber(bucket[1])
local last_time = tonumber(bucket[2])
if not tokens or not last_time then
    return { 'missing' }
end

local elapsed = nowInSeconds - last_time
local add_tokens = elapsed * rate
tokens = math.min(capacity, tokens + add_tokens)

return { 'present', tokens }

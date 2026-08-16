-- Record one request in the user's 60-second observation window.
-- KEYS[1]: urpm:v1:{uid}
-- ARGV[1]: request-id + ASCII unit separator + downstream model name

local now = redis.call('TIME')
local nowMs = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)

-- The closed lower boundary is removed on every write, matching inspect.lua.
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', nowMs - 60000)
if redis.call('ZADD', KEYS[1], 'NX', nowMs, ARGV[1]) == 1 then
    redis.call('PEXPIRE', KEYS[1], 65000)
end
return 1

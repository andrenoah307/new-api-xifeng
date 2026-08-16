-- Inspect one user's per-model request counts.
-- KEYS[1]: urpm:v1:{uid}
-- ARGV[1]: maximum members to scan

local now = redis.call('TIME')
local nowMs = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
local lower = nowMs - 60000

-- Trimming is the one intentional write in the read path. It bounds stale
-- members before ZCARD and keeps the closed boundary consistent with record.
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', lower)
if redis.call('ZCARD', KEYS[1]) > tonumber(ARGV[1]) then
    return { 'overflow' }
end

local members = redis.call('ZRANGEBYSCORE', KEYS[1], '(' .. lower, nowMs)
local separator = string.char(31)
local counts = {}
for _, member in ipairs(members) do
    local position = string.find(member, separator, 1, true)
    if position then
        local model = string.sub(member, position + 1)
        if model ~= '' then
            counts[model] = (counts[model] or 0) + 1
        end
    end
end

local result = {}
for model, count in pairs(counts) do
    table.insert(result, model)
    table.insert(result, count)
end
return result

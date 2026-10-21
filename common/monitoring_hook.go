package common

var GroupMonitoringHook func(group string, channelId, userId int, isSuccess bool, promptTokens, cacheTokens, useTimeMs, frtMs int, modelName string, statusCode int, content string, isStream bool)

package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// trustRecheckMultiplier bounds the cache values that may bypass an
// authoritative wallet read. It is a named policy constant, not a drift limit.
const trustRecheckMultiplier = 2

// readAuthoritativeUserQuota returns a wallet quota and whether the value was
// read from the database during this call. Cache values in the trust decision
// band are rechecked so stale-low values cannot bypass or trigger the wrong
// wallet gate.
func readAuthoritativeUserQuota(userId int) (quota int, freshFromDB bool, err error) {
	cacheQuota, source, err := model.GetUserQuotaWithSource(userId, false)
	if err != nil {
		return 0, false, err
	}
	if source == model.UserQuotaSourceDB {
		return cacheQuota, true, nil
	}
	if cacheQuota > trustRecheckMultiplier*common.GetTrustQuota() {
		return cacheQuota, false, nil
	}

	dbQuota, err := model.GetUserQuota(userId, true)
	if err != nil {
		return 0, false, err
	}
	if dbQuota != cacheQuota {
		common.SysLog(fmt.Sprintf("user quota cache drift user_id=%d cache=%d db=%d delta=%d", userId, cacheQuota, dbQuota, dbQuota-cacheQuota))
	}
	return dbQuota, true, nil
}

// ReadAuthoritativeUserQuota exposes the bounded wallet read to relay
// packages while keeping the policy implementation in this service package.
func ReadAuthoritativeUserQuota(userId int) (int, bool, error) {
	return readAuthoritativeUserQuota(userId)
}

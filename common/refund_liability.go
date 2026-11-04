package common

import "sync/atomic"

const RefundLiabilityContextKey = "refund_liability"

// Durable records survive process loss; these flags prevent premature cleanup.
type RefundLiability struct {
	ID     string
	Failed atomic.Bool
}

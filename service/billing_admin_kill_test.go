package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An administrator clearing a stalled connection does not touch billing code at
// all: it cancels the request context, the attempt fails, and the ordinary error
// path refunds. These tests pin the two money-losing outcomes that a kill could
// otherwise produce — a second refund for the same pre-consumption, and a refund
// issued for a request that already settled.

type fakeFundingSource struct {
	source     string
	refunds    atomic.Int32
	settles    atomic.Int32
	lastDelta  atomic.Int32
	refundDone chan struct{}
	once       sync.Once
}

func newFakeFundingSource() *fakeFundingSource {
	return &fakeFundingSource{source: BillingSourceWallet, refundDone: make(chan struct{})}
}

func (f *fakeFundingSource) Source() string { return f.source }

func (f *fakeFundingSource) PreConsume(amount int) error { return nil }

func (f *fakeFundingSource) Settle(delta int) error {
	f.settles.Add(1)
	f.lastDelta.Store(int32(delta))
	return nil
}

func (f *fakeFundingSource) Refund() error {
	f.refunds.Add(1)
	f.once.Do(func() { close(f.refundDone) })
	return nil
}

// waitForRefund waits for the asynchronous refund goroutine, bounded so a
// regression fails the test instead of hanging the suite.
func (f *fakeFundingSource) waitForRefund(t *testing.T) {
	t.Helper()
	select {
	case <-f.refundDone:
	case <-time.After(5 * time.Second):
		t.Fatal("funding refund never ran")
	}
}

// newKilledRequestSession builds a session in the state an admin kill finds it:
// pre-consumption already applied, nothing settled yet. IsPlayground keeps the
// token half of the refund out of the database; the funding half is what the
// counters here observe.
func newKilledRequestSession(preConsumed int) (*BillingSession, *fakeFundingSource) {
	funding := newFakeFundingSource()
	session := &BillingSession{
		relayInfo:        &relaycommon.RelayInfo{UserId: 1, TokenId: 2, IsPlayground: true},
		funding:          funding,
		preConsumedQuota: preConsumed,
		tokenConsumed:    preConsumed,
	}
	session.relayInfo.Billing = session
	return session, funding
}

func TestAdminKillRefundsThePreConsumptionExactlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	session, funding := newKilledRequestSession(1000)
	require.True(t, session.NeedsRefund())

	// The relay defer can run more than once across nested error paths, and the
	// realtime kill may land while it does. Every extra call must be a no-op.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.relayInfo.Billing.Refund(c)
		}()
	}
	wg.Wait()
	funding.waitForRefund(t)

	assert.EqualValues(t, 1, funding.refunds.Load(), "the pre-consumption may only be returned once")
	assert.EqualValues(t, 0, funding.settles.Load(), "a killed request never settles")
	assert.False(t, session.NeedsRefund(), "a refunded session no longer needs a refund")
}

func TestAdminKillAfterSettlementDoesNotRefundTheSettledCharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	// A kill that races a completed response arrives after Settle: the user has
	// been charged for real usage, so refunding now would hand back a live charge.
	session, funding := newKilledRequestSession(1000)
	require.NoError(t, session.Settle(1500))
	require.EqualValues(t, 1, funding.settles.Load())
	require.EqualValues(t, 500, funding.lastDelta.Load())

	assert.False(t, session.NeedsRefund())
	session.relayInfo.Billing.Refund(c)
	assert.EqualValues(t, 0, funding.refunds.Load(), "a settled request must not be refunded")

	// A repeated settlement would charge the delta twice.
	require.NoError(t, session.Settle(1500))
	assert.EqualValues(t, 1, funding.settles.Load(), "settlement is idempotent")
}

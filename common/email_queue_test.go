package common

import (
	"context"
	"net/textproto"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllEmailFormatsReuseOneAuthenticatedConnection(t *testing.T) {
	withSMTPSettings(t)
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	SMTPServer, SMTPPort = server.host, server.port
	SMTPAccount, SMTPToken, SMTPFrom = "sender@example.test", "secret", "sender@example.test"
	SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPForceAuthLogin = false, false, false
	require.NoError(t, SendEmail("Verification", "one@example.test", "code"))
	assert.Contains(t, <-server.messages, "To: one@example.test")
	select {
	case <-server.authCommands:
	default:
		t.Fatal("missing login")
	}
	state, err := SendRevenueEmail(context.Background(), "Report", "two@example.test", "report")
	require.NoError(t, err)
	assert.Equal(t, "accepted", state)
	assert.Contains(t, <-server.messages, "To: two@example.test")
	uncertain, err := SendEmailAttachments("Invoice", "three@example.test", "invoice", []EmailAttachment{{Name: "1.png", Data: []byte{1}}})
	require.NoError(t, err)
	assert.False(t, uncertain)
	assert.Contains(t, <-server.messages, "To: three@example.test")
	select {
	case <-server.authCommands:
		t.Fatal("another mail module logged in again")
	default:
	}
}

func TestEmailSharedCooldownAndExpiredLeaseFencing(t *testing.T) {
	withSMTPSettings(t)
	oldRDB := RDB
	mr := miniredis.RunT(t)
	RDB, RedisEnabled = redis.NewClient(&redis.Options{Addr: mr.Addr()}), true
	t.Cleanup(func() { _ = RDB.Close(); RDB = oldRDB })
	ctx := context.Background()
	lease, err := AcquireEmailLease(ctx, false)
	require.NoError(t, err)
	require.NoError(t, lease.Validate(ctx))
	_, err = AcquireEmailLease(ctx, false)
	require.ErrorIs(t, err, ErrEmailQueueBusy)
	lease.Release(&SMTPDeliveryError{Stage: "auth", Err: &textproto.Error{Code: 535, Msg: "limited"}})
	assert.Equal(t, 15*time.Minute, mr.TTL(EmailSMTPKey()))
	_, err = AcquireEmailLease(ctx, false)
	require.ErrorIs(t, err, ErrEmailQueueBusy)
	mr.FastForward(15 * time.Minute)
	current, err := AcquireEmailLease(ctx, false)
	require.NoError(t, err)
	lease.Release(nil)
	require.NoError(t, current.Validate(ctx), "expired owner cannot release another sender")
	require.ErrorIs(t, lease.Validate(ctx), ErrEmailQueueBusy)
	mr.FlushAll()
	require.ErrorIs(t, current.Validate(ctx), ErrEmailQueueBusy)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = AcquireEmailLease(cancelled, true)
	require.Error(t, err)
}

func TestEmailForegroundWaiterTakesPriorityOverBatch(t *testing.T) {
	withSMTPSettings(t)
	oldRDB := RDB
	mr := miniredis.RunT(t)
	RDB, RedisEnabled = redis.NewClient(&redis.Options{Addr: mr.Addr()}), true
	t.Cleanup(func() { _ = RDB.Close(); RDB = oldRDB })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := AcquireEmailLease(ctx, false)
	require.NoError(t, err)
	type outcome struct {
		lease *EmailLease
		err   error
	}
	done := make(chan outcome, 1)
	go func() { lease, err := AcquireEmailLease(ctx, true); done <- outcome{lease, err} }()
	require.Eventually(t, func() bool { return RDB.ZCard(ctx, EmailSMTPKey()+":waiting").Val() == 1 }, time.Second, 5*time.Millisecond)
	mr.FastForward(2 * time.Minute)
	_, err = AcquireEmailLease(ctx, false)
	require.ErrorIs(t, err, ErrEmailQueueBusy, "batch cannot take an urgent caller's slot")
	result := <-done
	require.NoError(t, result.err)
	require.NoError(t, result.lease.Validate(ctx))
	assert.Zero(t, RDB.ZCard(ctx, EmailSMTPKey()+":waiting").Val())
}

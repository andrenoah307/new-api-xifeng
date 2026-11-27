package common

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

var ErrEmailQueueBusy = errors.New("email delivery is temporarily unavailable")
var emailPacingInterval = time.Second
var sharedEmailSession smtpMailSession
var localEmailGate struct {
	sync.Mutex
	token string
	until time.Time
}

func ValidateEmailMessage(subject, receiver, content string) error {
	if strings.ContainsAny(subject+receiver, "\r\n") || len(subject) > 4096 || len(content) > 4<<20 {
		return errors.New("invalid email message")
	}
	recipients := strings.Split(receiver, ";")
	if len(recipients) > 50 {
		return errors.New("invalid email recipients")
	}
	for _, recipient := range recipients {
		address, err := mail.ParseAddress(recipient)
		if err != nil || address.Address != recipient {
			return errors.New("invalid email recipient")
		}
	}
	from := SMTPFrom
	if from == "" {
		from = SMTPAccount
	}
	address, err := mail.ParseAddress(from)
	if err != nil || address.Address != from || strings.ContainsAny(from, "\r\n") || SMTPServer == "" || SMTPPort <= 0 || SMTPPort > 65535 {
		return errors.New("invalid SMTP configuration")
	}
	return nil
}

// EmailLease is shared by all SMTP producers, including invoice and report
// workers. Redis contains coordination IDs only, never messages or credentials.
type EmailLease struct {
	key, token string
	local      bool
}

func EmailSMTPKey() string {
	account := SMTPAccount
	if account == "" {
		account = SMTPFrom
	}
	return fmt.Sprintf("mail:smtp:%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", SMTPServer, SMTPPort, account))))
}

var emailAcquire = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE',KEYS[2],'-inf',ARGV[2])
local waiting=redis.call('ZRANGE',KEYS[2],0,0)
if #waiting > 0 and waiting[1] ~= ARGV[1] then return 0 end
if redis.call('SET',KEYS[1],ARGV[1],'NX','PX',120000) then
 redis.call('ZREM',KEYS[2],ARGV[1]); return 1
end
return 0`)
var emailRelease = redis.NewScript(`if redis.call('GET',KEYS[1]) == ARGV[1] then redis.call('SET',KEYS[1],'cooldown','PX',ARGV[2]); return 1 end return 0`)
var emailLeaseCheck = redis.NewScript(`if redis.call('GET',KEYS[1]) == ARGV[1] and redis.call('PTTL',KEYS[1]) >= 45000 then return 1 end return 0`)

// Waiting callers (verification/test/report mail) have priority over background
// batch workers. Context expiry removes the waiter without sending anything.
func AcquireEmailLease(ctx context.Context, wait bool) (*EmailLease, error) {
	key, token := EmailSMTPKey(), uuid.NewString()
	local := !RedisEnabled
	if !local && RDB == nil {
		return nil, ErrEmailQueueBusy
	}
	if !local && wait {
		deadline := time.Now().Add(2 * time.Minute)
		if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
			deadline = end
		}
		if err := RDB.ZAdd(ctx, key+":waiting", &redis.Z{Score: float64(deadline.UnixMilli()), Member: token}).Err(); err != nil {
			return nil, ErrEmailQueueBusy
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = RDB.ZRem(cleanup, key+":waiting", token).Err()
		}()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		acquired := false
		if local {
			localEmailGate.Lock()
			if !time.Now().Before(localEmailGate.until) {
				localEmailGate.token, localEmailGate.until = token, time.Now().Add(2*time.Minute)
				acquired = true
			}
			localEmailGate.Unlock()
		} else {
			result, err := emailAcquire.Run(ctx, RDB, []string{key, key + ":waiting"}, token, time.Now().UnixMilli()).Int()
			if err != nil {
				return nil, ErrEmailQueueBusy
			}
			acquired = result == 1
		}
		if acquired {
			return &EmailLease{key: key, token: token, local: local}, nil
		}
		if !wait {
			return nil, ErrEmailQueueBusy
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (lease *EmailLease) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease == nil || lease.key != EmailSMTPKey() {
		return ErrEmailQueueBusy
	}
	if lease.local {
		localEmailGate.Lock()
		defer localEmailGate.Unlock()
		if localEmailGate.token == lease.token && time.Until(localEmailGate.until) >= 45*time.Second {
			return nil
		}
		return ErrEmailQueueBusy
	}
	valid, err := emailLeaseCheck.Run(ctx, RDB, []string{lease.key}, lease.token).Int()
	if err != nil || valid != 1 {
		return ErrEmailQueueBusy
	}
	return nil
}

func (lease *EmailLease) Release(sendErr error) {
	if lease == nil {
		return
	}
	delay := emailPacingInterval
	if retry := SMTPRetryDelay(sendErr, 1); retry > delay {
		delay = retry
	}
	if lease.local {
		localEmailGate.Lock()
		defer localEmailGate.Unlock()
		if localEmailGate.token == lease.token {
			localEmailGate.until = time.Now().Add(delay)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := emailRelease.Run(ctx, RDB, []string{lease.key}, lease.token, max(delay.Milliseconds(), 1)).Err(); err != nil {
		SysError("SMTP pacing update failed: " + err.Error())
	}
}

func CloseEmailSession() { sharedEmailSession.Close() }

func SendEmailWithLease(ctx context.Context, lease *EmailLease, subject, receiver, content string, files []EmailAttachment) (bool, error) {
	return sharedEmailSession.send(ctx, subject, receiver, content, files, func() error { return lease.Validate(ctx) })
}

func sendQueuedSMTP(ctx context.Context, subject, receiver, content string, files []EmailAttachment) (uncertain bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	lease, err := AcquireEmailLease(ctx, true)
	if err != nil {
		return false, err
	}
	defer func() { lease.Release(err) }()
	return SendEmailWithLease(ctx, lease, subject, receiver, content, files)
}

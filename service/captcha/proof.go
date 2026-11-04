package captcha

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const Lifetime = 2 * time.Minute

var ErrInvalid = errors.New("captcha verification failed")

type Claim struct {
	Scene    string `json:"scene"`
	Browser  string `json:"browser"`
	Target   string `json:"target"`
	Revision string `json:"revision"`
}

func RandomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func Issue(ctx context.Context, store Store, kind string, claim Claim) (string, error) {
	token, err := RandomToken()
	if err != nil {
		return "", err
	}
	b, err := common.Marshal(claim)
	if err != nil {
		return "", err
	}
	if err = store.Put(ctx, kind+token, string(b), Lifetime); err != nil {
		return "", err
	}
	return token, nil
}
func ConsumeProof(ctx context.Context, store Store, token string, claim Claim) error {
	if len(token) != 64 {
		return ErrInvalid
	}
	b, err := common.Marshal(claim)
	if err != nil {
		return err
	}
	ok, err := store.Consume(ctx, "proof:"+token, string(b))
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalid
	}
	return nil
}
func ConsumeChallenge(ctx context.Context, store Store, token, browser, revision string) (Claim, error) {
	var claim Claim
	if len(token) != 64 {
		return claim, ErrInvalid
	}
	raw, err := store.Get(ctx, "challenge:"+token)
	if err != nil {
		return claim, err
	}
	if raw == "" || common.UnmarshalJsonStr(raw, &claim) != nil || claim.Browser != browser || claim.Revision != revision {
		return Claim{}, ErrInvalid
	}
	ok, err := store.Consume(ctx, "challenge:"+token, raw)
	if err != nil {
		return Claim{}, err
	}
	if !ok {
		return Claim{}, ErrInvalid
	}
	return claim, nil
}

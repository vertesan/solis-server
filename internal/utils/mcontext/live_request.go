package mcontext

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

type liveRequestKey struct{}

type liveRequest struct {
	seed      uint64
	startedAt time.Time
}

// WithLiveRequest chooses entropy and time once, before any retryable transaction.
// Calling it again on a transaction-derived context preserves the original values.
func WithLiveRequest(ctx context.Context) (context.Context, error) {
	if _, _, ok := GetLiveRequest(ctx); ok {
		return ctx, nil
	}
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, fmt.Errorf("create live request seed: %w", err)
	}
	return context.WithValue(ctx, liveRequestKey{}, liveRequest{
		seed: binary.LittleEndian.Uint64(entropy[:]), startedAt: time.Now(),
	}), nil
}

func GetLiveRequest(ctx context.Context) (uint64, time.Time, bool) {
	value, ok := ctx.Value(liveRequestKey{}).(liveRequest)
	return value.seed, value.startedAt, ok
}

// WithLiveRequestVariant derives an independent, retry-stable stream for one
// quest inside a bulk request while retaining the parent request's JST day/time.
func WithLiveRequestVariant(ctx context.Context, label string) (context.Context, error) {
	ctx, err := WithLiveRequest(ctx)
	if err != nil {
		return nil, err
	}
	seed, started, _ := GetLiveRequest(ctx)
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], seed)
	h := sha256.New()
	h.Write(encoded[:])
	h.Write([]byte(label))
	digest := h.Sum(nil)
	return context.WithValue(ctx, liveRequestKey{}, liveRequest{
		seed: binary.LittleEndian.Uint64(digest[:8]), startedAt: started,
	}), nil
}

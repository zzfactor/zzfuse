package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type embeddingLeaseContextKey struct{}

// A batch session owns heartbeat cancellation and drains it before releasing
// incomplete claims. Its mutex serializes renewal against receipt completion.
type embeddingLeaseSession struct {
	db     EmbeddingClaimDatabase
	ctx    context.Context
	cancel context.CancelCauseFunc
	lease  time.Duration
	mu     sync.Mutex
	claims map[string]EmbeddingClaim
	wg     sync.WaitGroup
}

func newEmbeddingLeaseSession(ctx context.Context, db EmbeddingClaimDatabase, lease time.Duration, stopped <-chan struct{}) (*embeddingLeaseSession, context.Context) {
	if lease <= 0 {
		lease = defaultEmbeddingLease
	}
	ctx, cancel := context.WithCancelCause(ctx)
	s := &embeddingLeaseSession{db: db, ctx: ctx, cancel: cancel, lease: lease, claims: make(map[string]EmbeddingClaim)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(max(lease/3, time.Millisecond))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopped:
				cancel(context.Canceled)
				return
			case <-ticker.C:
				s.mu.Lock()
				claims := s.snapshot()
				if len(claims) > 0 {
					renewCtx, done := context.WithTimeout(ctx, max(lease/3, time.Millisecond))
					err := db.RenewEmbeddingClaims(renewCtx, claims, lease)
					done()
					if err != nil {
						cancel(fmt.Errorf("renew embedding lease: %w", err))
						s.mu.Unlock()
						return
					}
				}
				s.mu.Unlock()
			}
		}
	}()
	return s, context.WithValue(ctx, embeddingLeaseContextKey{}, s)
}

func embeddingSession(ctx context.Context) *embeddingLeaseSession {
	s, _ := ctx.Value(embeddingLeaseContextKey{}).(*embeddingLeaseSession)
	return s
}

func (s *embeddingLeaseSession) snapshot() []EmbeddingClaim {
	claims := make([]EmbeddingClaim, 0, len(s.claims))
	for _, claim := range s.claims {
		claims = append(claims, claim)
	}
	return claims
}
func (s *embeddingLeaseSession) add(claims []EmbeddingClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, claim := range claims {
		s.claims[claimKey(claim)] = claim
	}
}
func (s *embeddingLeaseSession) close() {
	s.cancel(context.Canceled)
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.db.ReleaseEmbeddingClaims(ctx, s.snapshot())
}

func (s *embeddingLeaseSession) complete(ctx context.Context, signals []map[string]interface{}, vectors []string) error {
	claims := make([]EmbeddingClaim, 0, len(signals))
	for _, signal := range signals {
		claim, ok := signal[claimSignalKey].(EmbeddingClaim)
		if !ok {
			return fmt.Errorf("signal is missing its durable embedding claim")
		}
		claims = append(claims, claim)
	}
	if len(vectors) == 1 && len(claims) > 1 {
		vector := vectors[0]
		vectors = make([]string, len(claims))
		for i := range vectors {
			vectors[i] = vector
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.CompleteEmbeddingClaims(ctx, claims, vectors); err != nil {
		return err
	}
	for _, claim := range claims {
		delete(s.claims, claimKey(claim))
	}
	return nil
}

// Written vectors survive a lost receipt/crash. Finish those receipts before
// submitting only unwritten members to inference, including grouped retries.
func (s *embeddingLeaseSession) recover(ctx context.Context, signals []map[string]interface{}) ([]map[string]interface{}, error) {
	var pending []map[string]interface{}
	for _, signal := range signals {
		claim := signal[claimSignalKey].(EmbeddingClaim)
		if claim.VectorID > 0 {
			err := s.complete(ctx, []map[string]interface{}{signal}, []string{fmt.Sprint(claim.VectorID)})
			if err == nil {
				continue
			}
			if !errors.Is(err, ErrEmbeddingVectorMissing) {
				return nil, err
			}
			if err := s.db.ResetEmbeddingClaimVector(ctx, claim); err != nil {
				return nil, err
			}
			claim.VectorID = 0
			signal[claimSignalKey] = claim
		}
		pending = append(pending, signal)
	}
	return pending, nil
}

func attachEmbeddingFences(metadata map[string]interface{}, signals []map[string]interface{}) {
	var claims []EmbeddingClaim
	for _, signal := range signals {
		if claim, ok := signal[claimSignalKey].(EmbeddingClaim); ok {
			claims = append(claims, claim)
		}
	}
	if len(claims) > 0 {
		metadata[claimMetadataKey] = claims
	}
}

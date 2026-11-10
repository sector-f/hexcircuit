package circuit

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultGenerationAttempts(t *testing.T) {
	if got := DefaultOptions().GenerationAttempts; got != 4 {
		t.Fatalf("default generation attempts = %d, want 4", got)
	}
}

func TestNormalizeOptionsRejectsInvalidGenerationAttempts(t *testing.T) {
	for _, attempts := range []int{0, -1, maxGenerationAttempts + 1} {
		opts := DefaultOptions()
		opts.GenerationAttempts = attempts
		if _, err := normalizeOptions(opts); err == nil {
			t.Errorf("generation attempts %d should be rejected", attempts)
		}
	}
}

func TestGrowClustersMarksRandomPlacementFailureRetryable(t *testing.T) {
	_, err := growClusters(
		[]Coord{{Q: 0, R: 0}},
		[]Coord{{Q: 0, R: 0}},
		2,
		rand.New(rand.NewSource(1)),
	)
	if err == nil {
		t.Fatal("expected cluster growth to fail when there are no eligible cells")
	}
	var retryable *retryableGenerationError
	if !errors.As(err, &retryable) {
		t.Fatalf("placement error %T is not marked retryable: %v", err, err)
	}
}

func TestGenerateWithSeedAttemptsRetriesRandomPlacementFailures(t *testing.T) {
	var seen []int64
	svg, accepted, candidateAttempts, err := generateWithSeedAttempts(3, 42, func(seed int64) (string, int, int, error) {
		seen = append(seen, seed)
		if len(seen) < 3 {
			return "", 0, 0, &retryableGenerationError{err: fmt.Errorf("placement exhausted")}
		}
		return "svg", 7, 11, nil
	})
	if err != nil {
		t.Fatalf("expected fallback seed to succeed: %v", err)
	}
	if svg != "svg" || accepted != 7 || candidateAttempts != 11 {
		t.Fatalf("result = (%q, %d, %d), want (%q, 7, 11)", svg, accepted, candidateAttempts, "svg")
	}
	want := []int64{42, generationAttemptSeed(42, 1), generationAttemptSeed(42, 2)}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("attempt seeds = %v, want %v", seen, want)
	}
}

func TestGenerateWithSeedAttemptsDoesNotRetryPermanentErrors(t *testing.T) {
	calls := 0
	permanentErr := fmt.Errorf("invalid geometry")
	_, _, _, err := generateWithSeedAttempts(4, 17, func(seed int64) (string, int, int, error) {
		calls++
		return "", 0, 0, permanentErr
	})
	if err != permanentErr {
		t.Fatalf("returned error = %v, want %v", err, permanentErr)
	}
	if calls != 1 {
		t.Fatalf("generator called %d times for a permanent error, want once", calls)
	}
}

func TestGenerateWithSeedAttemptsReportsExhaustion(t *testing.T) {
	calls := 0
	_, _, _, err := generateWithSeedAttempts(2, 17, func(seed int64) (string, int, int, error) {
		calls++
		return "", 0, 0, &retryableGenerationError{err: fmt.Errorf("placement exhausted")}
	})
	if err == nil {
		t.Fatal("expected an error after all fallback seeds fail")
	}
	if calls != 2 {
		t.Fatalf("generator called %d times, want 2", calls)
	}
	if !strings.Contains(err.Error(), "generation failed after 2 seed attempts") {
		t.Fatalf("error does not describe exhausted retries: %v", err)
	}
}

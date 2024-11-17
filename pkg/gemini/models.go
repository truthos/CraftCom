// Copyright (c) 2024 TruthOS
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package gemini

import (
	"craftcom/pkg/types"
	"sync"
	"time"
)

// RateLimiter handles API rate limiting for Gemini models
type RateLimiter struct {
	requestCount int           // Current request count
	tokenCount   int           // Current token count
	lastReset    time.Time     // Last minute reset time
	dailyCount   int           // Current daily request count
	dailyReset   time.Time     // Last daily reset time
	config       ModelConfig   // Associated model configuration
	usageHistory []UsageRecord // Track usage history
	mu           sync.Mutex    // Mutex for thread safety
}

// UsageRecord tracks API usage
type UsageRecord struct {
	Timestamp   time.Time
	RequestType string
	TokenCount  int
	Model       string
	Success     bool
	Error       error
}

// NewRateLimiter creates a new rate limiter for a specific model
func NewRateLimiter(config ModelConfig) *RateLimiter {
	return &RateLimiter{
		config:       config,
		lastReset:    time.Now(),
		dailyReset:   time.Now(),
		usageHistory: make([]UsageRecord, 0, 1000),
	}
}

// CheckLimit verifies if the operation is within rate limits
func (r *RateLimiter) CheckLimit() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Reset counters if needed
	if now.Sub(r.lastReset) >= time.Minute {
		r.resetMinuteCounts(now)
	}

	if now.Sub(r.dailyReset) >= 24*time.Hour {
		r.resetDailyCounts(now)
	}

	// Check limits
	if exceeded, msg := r.checkLimitExceeded(); exceeded {
		return types.NewCustomError(types.ErrRateLimit, msg, nil)
	}

	// Increment counters
	r.requestCount++
	r.dailyCount++

	return nil
}

// checkLimitExceeded checks if any limits are exceeded
func (r *RateLimiter) checkLimitExceeded() (bool, string) {
	if r.requestCount >= r.config.RPM {
		waitTime := time.Until(r.lastReset.Add(time.Minute))
		return true, types.ErrRateLimitf("RPM limit reached (%d/%d). Try again in %.0f seconds",
			r.requestCount, r.config.RPM, waitTime.Seconds()).Error()
	}

	if r.tokenCount >= r.config.TPM {
		waitTime := time.Until(r.lastReset.Add(time.Minute))
		return true, types.ErrRateLimitf("TPM limit reached (%d/%d). Try again in %.0f seconds",
			r.tokenCount, r.config.TPM, waitTime.Seconds()).Error()
	}

	if r.dailyCount >= r.config.RPD {
		waitTime := time.Until(r.dailyReset.Add(24 * time.Hour))
		return true, types.ErrRateLimitf("Daily limit reached (%d/%d). Try again in %.0f hours",
			r.dailyCount, r.config.RPD, waitTime.Hours()).Error()
	}

	return false, ""
}

// TrackTokens updates token usage count
func (r *RateLimiter) TrackTokens(count int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tokenCount += count
	r.recordUsage("token_update", count, true, nil)

	if r.tokenCount >= r.config.TPM {
		return types.ErrRateLimitf("Token limit exceeded: %d/%d tokens per minute",
			r.tokenCount, r.config.TPM)
	}

	return nil
}

// GetUsage returns current usage statistics
func (r *RateLimiter) GetUsage() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	minuteReset := r.lastReset.Add(time.Minute)
	dailyReset := r.dailyReset.Add(24 * time.Hour)

	return map[string]interface{}{
		"current": map[string]interface{}{
			"requests_per_minute": r.requestCount,
			"tokens_per_minute":   r.tokenCount,
			"requests_per_day":    r.dailyCount,
		},
		"limits": map[string]interface{}{
			"rpm": r.config.RPM,
			"tpm": r.config.TPM,
			"rpd": r.config.RPD,
		},
		"reset_times": map[string]interface{}{
			"minute_reset_in": time.Until(minuteReset).Seconds(),
			"daily_reset_in":  time.Until(dailyReset).Hours(),
		},
		"usage_percent": map[string]interface{}{
			"rpm": float64(r.requestCount) / float64(r.config.RPM) * 100,
			"tpm": float64(r.tokenCount) / float64(r.config.TPM) * 100,
			"rpd": float64(r.dailyCount) / float64(r.config.RPD) * 100,
		},
	}
}

// Reset resets all counters
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.resetMinuteCounts(now)
	r.resetDailyCounts(now)
	r.recordUsage("manual_reset", 0, true, nil)
}

// Internal helper functions
func (r *RateLimiter) resetMinuteCounts(now time.Time) {
	// Only reset if we've passed the minute boundary
	if now.Sub(r.lastReset) >= time.Minute {
		r.requestCount = 0
		r.tokenCount = 0
		r.lastReset = now.Truncate(time.Minute)
		r.recordUsage("minute_reset", 0, true, nil)
	}
}

func (r *RateLimiter) resetDailyCounts(now time.Time) {
	r.dailyCount = 0
	r.dailyReset = now
	r.recordUsage("daily_reset", 0, true, nil)
}

func (r *RateLimiter) recordUsage(requestType string, tokenCount int, success bool, err error) {
	record := UsageRecord{
		Timestamp:   time.Now(),
		RequestType: requestType,
		TokenCount:  tokenCount,
		Model:       r.config.Name,
		Success:     success,
		Error:       err,
	}

	r.usageHistory = append(r.usageHistory, record)

	// Keep only last 1000 records
	if len(r.usageHistory) > 1000 {
		r.usageHistory = r.usageHistory[1:]
	}
}

// WaitForAvailability waits until rate limits reset
func (r *RateLimiter) WaitForAvailability(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := r.CheckLimit(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return types.ErrTimeoutf("timeout waiting for rate limit reset after %v", timeout)
}

// GetRemainingQuota returns remaining quotas
func (r *RateLimiter) GetRemainingQuota() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return map[string]int{
		"remaining_rpm": r.config.RPM - r.requestCount,
		"remaining_tpm": r.config.TPM - r.tokenCount,
		"remaining_rpd": r.config.RPD - r.dailyCount,
	}
}

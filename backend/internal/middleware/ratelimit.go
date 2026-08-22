package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"golang.org/x/time/rate"
)

// clientLimiter wraps a token bucket rate.Limiter with last seen timestamp for cleanup.
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages per-IP rate limiters using token bucket algorithm.
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*clientLimiter
	rate     rate.Limit
	burst    int
}

// NewIPRateLimiter creates a new IPRateLimiter.
// requestsPerHour specifies the sustained rate per hour (e.g. 60/h).
// burst specifies the maximum burst allowed.
func NewIPRateLimiter(requestsPerHour int, burst int) *IPRateLimiter {
	if requestsPerHour <= 0 {
		requestsPerHour = 60
	}
	if burst <= 0 {
		burst = 10
	}

	limit := rate.Every(time.Hour / time.Duration(requestsPerHour))

	rl := &IPRateLimiter{
		limiters: make(map[string]*clientLimiter),
		rate:     limit,
		burst:    burst,
	}

	// Start background cleanup for stale IP entries every 10 minutes
	go rl.cleanupRoutine(10 * time.Minute)

	return rl
}

func (rl *IPRateLimiter) cleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		rl.cleanup(time.Now())
	}
}

func (rl *IPRateLimiter) cleanup(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, cl := range rl.limiters {
		if now.Sub(cl.lastSeen) > 2*time.Hour {
			delete(rl.limiters, ip)
		}
	}
}

// getLimiter returns or creates a rate.Limiter for the given IP address.
func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cl, exists := rl.limiters[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = &clientLimiter{
			limiter:  limiter,
			lastSeen: now,
		}
		return limiter
	}

	cl.lastSeen = now
	return cl.limiter
}

// ExtractIP extracts the client IP address from the request.
func ExtractIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	// Check X-Real-IP header
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}

	// Fallback to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimitMiddleware creates an HTTP middleware that rate limits requests per client IP.
// When rate limit is exceeded, it responds with HTTP 429 and standard Retry-After header.
func RateLimitMiddleware(rl *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ExtractIP(r)
			limiter := rl.getLimiter(ip)

			reservation := limiter.Reserve()
			if !reservation.OK() {
				// Cannot be reserved at all
				w.Header().Set("Retry-After", "60")
				errors.WriteCode(w, errors.CodeRateLimited)
				return
			}

			delay := reservation.Delay()
			if delay == 0 {
				// Token available immediately
				next.ServeHTTP(w, r)
				return
			}

			// Token not immediately available -> limit exceeded
			reservation.Cancel()

			retryAfterSeconds := int(math.Ceil(delay.Seconds()))
			if retryAfterSeconds <= 0 {
				retryAfterSeconds = 1
			}

			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
			errors.WriteCode(w, errors.CodeRateLimited)
		})
	}
}

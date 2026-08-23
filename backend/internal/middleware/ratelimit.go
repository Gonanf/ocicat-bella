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

// Allow consume un token para la clave dada (IP, email, etc.) y devuelve
// (retryAfterSegundos, ok). No escribe la respuesta HTTP.
func (rl *IPRateLimiter) Allow(key string) (retryAfter int, ok bool) {
	limiter := rl.getLimiter(key)

	reservation := limiter.Reserve()
	if !reservation.OK() {
		return 60, false
	}

	delay := reservation.Delay()
	if delay == 0 {
		return 0, true
	}
	reservation.Cancel()

	seconds := int(math.Ceil(delay.Seconds()))
	if seconds <= 0 {
		seconds = 1
	}
	return seconds, false
}

// WriteTooManyRequests responde 429 con Retry-After según §0.3.
func WriteTooManyRequests(w http.ResponseWriter, retryAfterSeconds int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	errors.WriteCode(w, errors.CodeRateLimited)
}

// FailLimiter limita FALLOS (no requests) por clave arbitraria, p.ej. login password:
// N fallos por ventana con recarga gradual ([C-rate] §0.3: 5 fallos/15min por email+IP).
// Los aciertos no consumen cupo.
type FailLimiter struct {
	mu       sync.Mutex
	entries  map[string]*clientLimiter
	attempts int
	window   time.Duration
}

// NewFailLimiter crea un FailLimiter de maxAttempts fallos por window.
func NewFailLimiter(maxAttempts int, window time.Duration) *FailLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	fl := &FailLimiter{
		entries:  make(map[string]*clientLimiter),
		attempts: maxAttempts,
		window:   window,
	}
	go func() {
		ticker := time.NewTicker(window)
		for range ticker.C {
			now := time.Now()
			fl.mu.Lock()
			for k, e := range fl.entries {
				if now.Sub(e.lastSeen) > 2*window {
					delete(fl.entries, k)
				}
			}
			fl.mu.Unlock()
		}
	}()
	return fl
}

// RecordFailure registra un fallo; devuelve false si se superó el límite.
func (fl *FailLimiter) RecordFailure(key string) bool {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	e, exists := fl.entries[key]
	if !exists {
		e = &clientLimiter{limiter: rate.NewLimiter(rate.Every(fl.window/time.Duration(fl.attempts)), fl.attempts)}
		fl.entries[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter.Allow()
}

// RateLimitMiddleware creates an HTTP middleware that rate limits requests per client IP.
// When rate limit is exceeded, it responds with HTTP 429 and standard Retry-After header.
func RateLimitMiddleware(rl *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			retryAfter, ok := rl.Allow(ExtractIP(r))
			if !ok {
				WriteTooManyRequests(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

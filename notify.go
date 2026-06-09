package main

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// Desktop notifications via notify-send (Linux)
// ────────────────────────────────────────────────────────────────────────────

// NotifyLevel controls the urgency of a desktop notification.
type NotifyLevel int

const (
	// NotifyCritical — high urgency, persists until dismissed.
	NotifyCritical NotifyLevel = iota
	// NotifyWarning — normal urgency.
	NotifyWarning
	// NotifyInfo — low urgency, auto-dismisses.
	NotifyInfo
)

func (l NotifyLevel) urgency() string {
	switch l {
	case NotifyCritical:
		return "critical"
	case NotifyWarning:
		return "normal"
	default:
		return "low"
	}
}

// Notifier sends debounced desktop notifications.
// Duplicate notifications (same key within cooldown) are silently dropped.
type Notifier struct {
	mu       sync.Mutex
	lastSent map[string]time.Time
	cooldown time.Duration
}

// NewNotifier creates a notifier with a 30-second cooldown.
func NewNotifier() *Notifier {
	return &Notifier{
		lastSent: make(map[string]time.Time),
		cooldown: 30 * time.Second,
	}
}

// Send dispatches a desktop notification if the cooldown for the given key
// has elapsed. key is used for deduplication (e.g. "slave_auth_12345").
func (n *Notifier) Send(key string, level NotifyLevel, title, message string) {
	n.mu.Lock()
	if last, ok := n.lastSent[key]; ok && time.Since(last) < n.cooldown {
		n.mu.Unlock()
		return
	}
	n.lastSent[key] = time.Now()
	n.mu.Unlock()

	go n.sendDesktop(level, title, message)
}

// Clear resets the cooldown for a key, allowing the next Send to fire immediately.
func (n *Notifier) Clear(key string) {
	n.mu.Lock()
	delete(n.lastSent, key)
	n.mu.Unlock()
}

func (n *Notifier) sendDesktop(level NotifyLevel, title, message string) {
	// notify-send — available on most Linux desktops
	exec.Command("notify-send",
		"-u", level.urgency(),
		"-a", "FundingPips Copier",
		title,
		message,
	).Run() // best-effort; silence error if notify-send isn't available
}

// isRateLimit checks if an error likely indicates API rate-limiting.
func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate_limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "throttl") ||
		strings.Contains(msg, "slow down")
}

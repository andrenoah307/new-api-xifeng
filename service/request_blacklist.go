package service

import (
	"container/list"
	"fmt"
	"sync"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

const requestBlacklistWindowStep = 32 * 1024

type BlacklistDecision struct {
	Hit       bool
	MatchKind string
	snapshot  *setting.RequestBlacklistSnapshot
}

func (d BlacklistDecision) Mode() string {
	if d.snapshot == nil {
		return setting.RequestBlacklistModeOff
	}
	return d.snapshot.Mode()
}

func (d BlacklistDecision) IsEnforced() bool {
	return d.Hit && d.Mode() == setting.RequestBlacklistModeEnforce
}

func (d BlacklistDecision) BlockMessage() string {
	if d.snapshot == nil {
		return ""
	}
	return d.snapshot.BlockMessage()
}

func (d BlacklistDecision) BlockStatusCode() int {
	if d.snapshot == nil {
		return 403
	}
	return d.snapshot.BlockStatusCode()
}

func (d BlacklistDecision) ConfigHash() string {
	if d.snapshot == nil {
		return ""
	}
	return d.snapshot.ConfigHash()
}

var requestBlacklistRunePool sync.Pool

func InspectRequestBlacklist(group, text string) BlacklistDecision {
	snapshot := setting.GetRequestBlacklistSnapshot()
	if text == "" || !snapshot.ShouldInspectGroup(group) {
		return BlacklistDecision{}
	}
	decision, _ := inspectRequestBlacklistText(snapshot, text, true)
	return decision
}

func inspectRequestBlacklistText(snapshot *setting.RequestBlacklistSnapshot, text string, stopAfterFirst bool) (BlacklistDecision, int) {
	if text == "" || snapshot == nil || snapshot.Machine() == nil || snapshot.MaxPatternRunes() == 0 {
		return BlacklistDecision{}, 0
	}

	pad := snapshot.MaxPatternRunes() + 1
	normalCapacity := requestBlacklistWindowStep + 2*pad
	buffer := acquireRequestBlacklistRuneBuffer(normalCapacity)
	defer func() {
		if cap(buffer) == normalCapacity {
			requestBlacklistRunePool.Put(buffer[:0])
		}
	}()

	windowIndex := 0
	windowStart := 0
	totalRunes := 0
	accepted := 0
	firstDecision := BlacklistDecision{}
	processWindow := func(coreEnd int) bool {
		coreStart := windowIndex * requestBlacklistWindowStep
		decision, count := inspectRequestBlacklistWindow(snapshot, buffer, windowStart, coreStart, coreEnd, stopAfterFirst)
		accepted += count
		if !firstDecision.Hit && decision.Hit {
			firstDecision = decision
		}
		return stopAfterFirst && decision.Hit
	}

	for _, r := range text {
		buffer = append(buffer, unicode.ToLower(r))
		totalRunes++
		windowTargetEnd := (windowIndex+1)*requestBlacklistWindowStep + pad
		if totalRunes < windowTargetEnd {
			continue
		}

		if processWindow((windowIndex + 1) * requestBlacklistWindowStep) {
			return firstDecision, accepted
		}
		nextWindowStart := (windowIndex+1)*requestBlacklistWindowStep - pad
		retainOffset := nextWindowStart - windowStart
		copy(buffer, buffer[retainOffset:])
		buffer = buffer[:len(buffer)-retainOffset]
		windowIndex++
		windowStart = nextWindowStart
	}

	coreStart := windowIndex * requestBlacklistWindowStep
	if totalRunes > coreStart {
		processWindow(totalRunes)
	}
	return firstDecision, accepted
}

func acquireRequestBlacklistRuneBuffer(capacity int) []rune {
	if pooled := requestBlacklistRunePool.Get(); pooled != nil {
		buffer, ok := pooled.([]rune)
		if ok && cap(buffer) == capacity {
			return buffer[:0]
		}
	}
	return make([]rune, 0, capacity)
}

func inspectRequestBlacklistWindow(
	snapshot *setting.RequestBlacklistSnapshot,
	window []rune,
	windowStart int,
	coreStart int,
	coreEnd int,
	stopAfterFirst bool,
) (BlacklistDecision, int) {
	returnImmediately := stopAfterFirst && !snapshot.HasDomainPatterns()
	terms := snapshot.Machine().MultiPatternSearch(window, returnImmediately)
	if returnImmediately && len(terms) > 0 {
		absoluteStart := windowStart + terms[0].Pos
		if absoluteStart < coreStart || absoluteStart >= coreEnd {
			terms = snapshot.Machine().MultiPatternSearch(window, false)
		}
	}
	firstDecision := BlacklistDecision{}
	accepted := 0
	for _, term := range terms {
		absoluteStart := windowStart + term.Pos
		if absoluteStart < coreStart || absoluteStart >= coreEnd {
			continue
		}

		contentPattern, domainPattern := snapshot.PatternKinds(term.Word)
		matchKind := ""
		if contentPattern {
			matchKind = "content"
		} else if domainPattern && requestBlacklistDomainBoundaryMatches(window, term.Pos, term.Word) {
			matchKind = "domain"
		}
		if matchKind == "" {
			continue
		}

		accepted++
		if !firstDecision.Hit {
			firstDecision = BlacklistDecision{Hit: true, MatchKind: matchKind, snapshot: snapshot}
		}
		if stopAfterFirst {
			return firstDecision, accepted
		}
	}
	return firstDecision, accepted
}

func requestBlacklistDomainBoundaryMatches(window []rune, start int, pattern []rune) bool {
	leadingDotPattern := len(pattern) > 0 && pattern[0] == '.'
	if !leadingDotPattern && start > 0 {
		left := window[start-1]
		if isRequestBlacklistDomainLabelRune(left) || left == '.' {
			return false
		}
	}

	rightIndex := start + len(pattern)
	if rightIndex >= len(window) {
		return true
	}
	right := window[rightIndex]
	if isRequestBlacklistDomainLabelRune(right) {
		return false
	}
	if right == '.' && rightIndex+1 < len(window) && isRequestBlacklistDomainLabelRune(window[rightIndex+1]) {
		return false
	}
	return true
}

func isRequestBlacklistDomainLabelRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

type requestBlacklistAuditEntry struct {
	key       string
	expiresAt time.Time
}

type requestBlacklistAuditLimiter struct {
	mu              sync.Mutex
	capacity        int
	window          time.Duration
	entries         map[string]*list.Element
	order           list.List
	suppressed      uint64
	nextSummaryTime time.Time
}

func newRequestBlacklistAuditLimiter(capacity int, window time.Duration) *requestBlacklistAuditLimiter {
	return &requestBlacklistAuditLimiter{
		capacity: capacity,
		window:   window,
		entries:  make(map[string]*list.Element, capacity),
	}
}

func (l *requestBlacklistAuditLimiter) allow(key string, now time.Time) (bool, uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	suppressedSummary := uint64(0)
	if l.nextSummaryTime.IsZero() {
		l.nextSummaryTime = now.Add(l.window)
	} else if !now.Before(l.nextSummaryTime) {
		suppressedSummary = l.suppressed
		l.suppressed = 0
		l.nextSummaryTime = now.Add(l.window)
	}

	for element := l.order.Front(); element != nil; element = l.order.Front() {
		entry := element.Value.(*requestBlacklistAuditEntry)
		if now.Before(entry.expiresAt) {
			break
		}
		delete(l.entries, entry.key)
		l.order.Remove(element)
	}

	if _, exists := l.entries[key]; exists {
		l.suppressed++
		return false, suppressedSummary
	}
	if l.capacity <= 0 {
		return false, suppressedSummary
	}
	if len(l.entries) >= l.capacity {
		oldest := l.order.Front()
		entry := oldest.Value.(*requestBlacklistAuditEntry)
		delete(l.entries, entry.key)
		l.order.Remove(oldest)
	}
	entry := &requestBlacklistAuditEntry{key: key, expiresAt: now.Add(l.window)}
	l.entries[key] = l.order.PushBack(entry)
	return true, suppressedSummary
}

func (l *requestBlacklistAuditLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

var globalRequestBlacklistAuditLimiter = newRequestBlacklistAuditLimiter(4096, time.Minute)

func AuditRequestBlacklistHit(c *gin.Context, decision BlacklistDecision) {
	if c == nil || !decision.Hit {
		return
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	identity := fmt.Sprintf("user:%d", userID)
	if tokenID > 0 {
		identity = fmt.Sprintf("token:%d", tokenID)
	}
	key := identity + ":" + decision.MatchKind
	logCurrent, suppressed := globalRequestBlacklistAuditLimiter.allow(key, time.Now())
	if suppressed > 0 {
		logger.LogWarn(c, fmt.Sprintf("request_blacklist_audit_suppressed count=%d window_seconds=%d", suppressed, int(time.Minute.Seconds())))
	}
	if !logCurrent {
		return
	}

	route := ""
	if c.Request != nil && c.Request.URL != nil {
		route = c.Request.URL.Path
	}
	logger.LogWarn(c, fmt.Sprintf(
		"request_blacklist_hit request_id=%q user_id=%d token_id=%d group=%q route=%q mode=%q match_kind=%q config_hash=%q",
		c.GetString(common.RequestIdKey),
		userID,
		tokenID,
		common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		route,
		decision.Mode(),
		decision.MatchKind,
		decision.ConfigHash(),
	))
}

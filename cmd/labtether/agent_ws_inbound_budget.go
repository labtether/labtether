package main

import "time"

const (
	defaultAgentWSMessagesPerSecond = 256
	defaultAgentWSMessageBurst      = 512
	hardAgentWSMessagesPerSecond    = 8192
	hardAgentWSMessageBurst         = 16384
	defaultAgentWSBytesPerSecond    = 16 << 20
	defaultAgentWSByteBurst         = 64 << 20
	hardAgentWSBytesPerSecond       = 256 << 20
	hardAgentWSByteBurst            = 1 << 30

	// maxAgentWSInboundWait bounds one throttle pause in the read loop. The
	// read deadline is 60 s and pongs to the 30 s pings are only read between
	// pauses, so a pause must stay well under 30 s. The largest frame (32 MiB)
	// at the default 16 MiB/s needs about 2 s.
	maxAgentWSInboundWait = 10 * time.Second
)

// agentWSInboundBudget is a message and byte token bucket for one agent
// socket. A frame that fits is charged at once, even when the bucket is short;
// the read loop then pauses until the debt is repaid, so TCP backpressure slows
// a fast agent instead of the Hub dropping its connection.
type agentWSInboundBudget struct {
	messagesPerSecond float64
	messageBurst      float64
	bytesPerSecond    float64
	byteBurst         float64
	messageTokens     float64
	byteTokens        float64
	lastRefill        time.Time
}

func newAgentWSInboundBudget() *agentWSInboundBudget {
	messagesPerSecond := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_MESSAGES_PER_SECOND", defaultAgentWSMessagesPerSecond, hardAgentWSMessagesPerSecond)
	messageBurst := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_MESSAGE_BURST", defaultAgentWSMessageBurst, hardAgentWSMessageBurst)
	bytesPerSecond := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_BYTES_PER_SECOND", defaultAgentWSBytesPerSecond, hardAgentWSBytesPerSecond)
	byteBurst := boundedAgentWSEnvInt("LABTETHER_AGENT_WS_BYTE_BURST", defaultAgentWSByteBurst, hardAgentWSByteBurst)
	return &agentWSInboundBudget{
		messagesPerSecond: float64(messagesPerSecond),
		messageBurst:      float64(messageBurst),
		bytesPerSecond:    float64(bytesPerSecond),
		byteBurst:         float64(byteBurst),
		messageTokens:     float64(messageBurst),
		byteTokens:        float64(byteBurst),
		lastRefill:        time.Now(),
	}
}

// reserve charges one frame and returns how long the read loop must pause
// before handling it. ok is false for a frame the budget can never pay for:
// larger than the byte burst, or needing a pause longer than
// maxAgentWSInboundWait at the configured rates.
func (b *agentWSInboundBudget) reserve(frameBytes int, now time.Time) (time.Duration, bool) {
	if b == nil || frameBytes < 0 || b.messagesPerSecond <= 0 || b.bytesPerSecond <= 0 {
		return 0, false
	}
	size := float64(frameBytes)
	if b.messageBurst < 1 || size > b.byteBurst {
		return 0, false
	}
	maxWait := maxAgentWSInboundWait.Seconds()
	if 1/b.messagesPerSecond > maxWait || size/b.bytesPerSecond > maxWait {
		return 0, false
	}
	if elapsed := now.Sub(b.lastRefill).Seconds(); elapsed > 0 {
		b.messageTokens = min(b.messageBurst, b.messageTokens+elapsed*b.messagesPerSecond)
		b.byteTokens = min(b.byteBurst, b.byteTokens+elapsed*b.bytesPerSecond)
		b.lastRefill = now
	}
	b.messageTokens--
	b.byteTokens -= size
	wait := max(agentWSTokenDebtWait(b.messageTokens, b.messagesPerSecond), agentWSTokenDebtWait(b.byteTokens, b.bytesPerSecond))
	return min(wait, maxAgentWSInboundWait), true
}

func agentWSTokenDebtWait(tokens, perSecond float64) time.Duration {
	if tokens >= 0 {
		return 0
	}
	return time.Duration(-tokens / perSecond * float64(time.Second))
}

// waitAgentWSInboundBudget pauses the read loop and reports false when the
// connection was retired during the pause.
func waitAgentWSInboundBudget(done <-chan struct{}, wait time.Duration) bool {
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}

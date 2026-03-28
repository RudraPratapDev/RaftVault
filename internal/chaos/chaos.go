package chaos

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

// ChaosModule provides fault injection capabilities
type ChaosModule struct {
	mu       sync.RWMutex
	killed   bool
	delayMs    int
	dropRate   float64 // 0.0 to 1.0
	partitions map[string]bool
}

// NewChaosModule creates a new ChaosModule
func NewChaosModule() *ChaosModule {
	return &ChaosModule{
		partitions: make(map[string]bool),
	}
}

// Kill simulates a node crash - all requests will return 503
func (c *ChaosModule) Kill() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.killed = true
	log.Printf("[CHAOS] 💀 Node KILLED - all requests will be rejected")
}

// Revive brings a killed node back online
func (c *ChaosModule) Revive() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.killed = false
	log.Printf("[CHAOS] ✅ Node REVIVED - accepting requests again")
}

// SetDelay sets an artificial delay (in ms) for all RPC responses
func (c *ChaosModule) SetDelay(ms int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delayMs = ms
	log.Printf("[CHAOS] ⏱️  Delay set to %dms", ms)
}

// SetDropRate sets the rate at which incoming RPCs are randomly dropped
func (c *ChaosModule) SetDropRate(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	c.dropRate = rate
	log.Printf("[CHAOS] 🎲 Drop rate set to %.1f%%", rate*100)
}

// IsKilled returns true if the node is in a killed state
func (c *ChaosModule) IsKilled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.killed
}

// Partition simulates a network partition from a target nodeID or address
func (c *ChaosModule) Partition(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.partitions[target] = true
	log.Printf("[CHAOS] ✂️  Partitioned from %s", target)
}

// Heal resolves a network partition
func (c *ChaosModule) Heal(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.partitions, target)
	log.Printf("[CHAOS] 🔗 Healed partition with %s", target)
}

// IsPartitioned returns true if partitioned from the target
func (c *ChaosModule) IsPartitioned(target string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.partitions[target]
}

// ShouldDrop returns true if this request should be randomly dropped
func (c *ChaosModule) ShouldDrop() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dropRate <= 0 {
		return false
	}
	return rand.Float64() < c.dropRate
}

// ApplyDelay applies the configured artificial delay
func (c *ChaosModule) ApplyDelay() {
	c.mu.RLock()
	delay := c.delayMs
	c.mu.RUnlock()
	if delay > 0 {
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

// GetStatus returns the current chaos configuration
func (c *ChaosModule) GetStatus() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// copy partitions
	parts := make([]string, 0, len(c.partitions))
	for k, v := range c.partitions {
		if v {
			parts = append(parts, k)
		}
	}

	return map[string]interface{}{
		"killed":     c.killed,
		"delay_ms":   c.delayMs,
		"drop_rate":  c.dropRate,
		"partitions": parts,
	}
}

package providers

import (
	"time"

	"github.com/vibexp/vibexp/internal/cache"
)

// ProvideCache creates a new in-memory cache with default TTL
func ProvideCache() cache.CacheInterface {
	return cache.NewSimpleCache(15 * time.Minute)
}

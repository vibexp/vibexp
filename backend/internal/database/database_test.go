package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
)

// TestBuildDSN_SSLModeThreadedIntoBothHostShapes pins that the configured
// sslmode reaches the DSN for both the TCP and Unix-socket (Cloud SQL) branches,
// and that the default "disable" reproduces the pre-#293 connection string.
func TestBuildDSN_SSLModeThreadedIntoBothHostShapes(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.DatabaseConfig
		want string
	}{
		{
			name: "TCP, sslmode disable (default, unchanged from before #293)",
			cfg: config.DatabaseConfig{
				Host: "localhost", Port: "5432", User: "u", Password: "p", Name: "db", SSLMode: "disable",
			},
			want: "host=localhost port=5432 user=u password=p dbname=db sslmode=disable",
		},
		{
			name: "TCP, sslmode require",
			cfg: config.DatabaseConfig{
				Host: "db.example.com", Port: "5432", User: "u", Password: "p", Name: "db", SSLMode: "require",
			},
			want: "host=db.example.com port=5432 user=u password=p dbname=db sslmode=require",
		},
		{
			name: "Unix socket (Cloud SQL) omits port and carries sslmode require",
			cfg: config.DatabaseConfig{
				Host: "/cloudsql/proj:region:inst", Port: "5432", User: "u", Password: "p", Name: "db", SSLMode: "require",
			},
			want: "host=/cloudsql/proj:region:inst user=u password=p dbname=db sslmode=require",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, buildDSN(tc.cfg))
		})
	}
}

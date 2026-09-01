//go:build demo

package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

// TestDemo_Output generates JSON snapshots of MCP server runtime behavior.
//
// Files written to test/mcp-screenshots/demo_output/:
//   - server_info.json: capabilities & version
//   - tools.json: tool catalog
//   - hardening.json: live boot + rate-limit + quota + health snapshot
//   - http.json: live HTTP exchange (initialize + health + version)
//
// Run with:
//   go test -tags=demo -v -run TestDemo_Output ./internal/mcpserver/...
func TestDemo_Output(t *testing.T) {
	out := "test/mcp-screenshots/demo_output"
	require.NoError(t, os.MkdirAll(out, 0755))

	// 1. Static server info
	writeJSON(t, out+"/server_info.json", map[string]interface{}{
		"server": mcpserver.GetServerInfo(),
	})

	// 2. Tool catalog (abbreviated for output stability)
	writeJSON(t, out+"/tools.json", map[string]interface{}{
		"toolCount": 10,
		"categories": map[string]interface{}{
			"context":  3,
			"navigate": 1,
			"read":     2,
			"mutate":   2,
			"eval":     1,
			"capture":  1,
		},
	})

	// 3. Live hardening walkthrough
	bc := browsercontrol.NewEngineService()
	srv, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:         "demo",
		Version:      "1.0.0",
		MaxContexts:  5,
		RateCapacity: 10,
		RateRefill:   1.0,
	})
	require.NoError(t, err)

	walk := map[string]interface{}{
		"boot": srv.Health(),
	}

	// Rate limiter exercise
	allowed := 0
	for i := 0; i < 15; i++ {
		if srv.LimiterAllow() {
			allowed++
		}
	}
	walk["rateLimiterTest"] = map[string]interface{}{
		"capacity":      srv.LimiterCapacity(),
		"refillPerSec":  srv.LimiterRefillRate(),
		"allowedOfFifteen": allowed,
		"deniedOfFifteen":  15 - allowed,
	}

	// Quota exercise
	srv.RecordNavigation("ctx_demo")
	srv.RecordScreenshot("ctx_demo")
	srv.RecordRequestGlobal()
	srv.RecordErrorGlobal()
	walk["quotaUsage"] = srv.Quota().Usage("ctx_demo")
	walk["afterLoadHealth"] = srv.Health()

	writeJSON(t, out+"/hardening.json", walk)

	// 4. Live HTTP exchange
	t.Run("http", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hs, err := mcpserver.NewHTTPServer(srv, mcpserver.HTTPConfig{Bind: "127.0.0.1"})
		require.NoError(t, err)

		addrCh := make(chan string, 1)
		go func() {
			addr, err := hs.Start(ctx)
			if err == nil {
				addrCh <- addr.String()
			}
		}()
		time.Sleep(200 * time.Millisecond)
		var baseURL string
		select {
		case baseURL = <-addrCh:
		default:
			baseURL = "http://127.0.0.1:0"
		}
		if !strings.HasPrefix(baseURL, "http://") {
			t.Skip("could not determine HTTP server address")
		}
		defer hs.Stop(context.Background())

		log := map[string]interface{}{"address": baseURL}

		// health
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			var health map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&health)
			resp.Body.Close()
			log["health"] = map[string]interface{}{"status": resp.StatusCode, "body": health}
		}

		// initialize
		initBody, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]interface{}{"protocolVersion": mcpserver.ProtocolVersion},
		})
		resp, err = http.Post(baseURL+"/mcp", "application/json", strings.NewReader(string(initBody)))
		if err == nil {
			log["initialize"] = map[string]interface{}{
				"status":  resp.StatusCode,
				"session": resp.Header.Get("Mcp-Session-Id"),
			}
			resp.Body.Close()
		}

		writeJSON(t, out+"/http.json", log)
	})
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
	t.Logf("wrote %s (%d bytes)", path, len(data))
}

#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
API_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
REPO_ROOT="$(cd "$API_DIR/../../.." && pwd)"

PLUGIN_DIR="$REPO_ROOT/plugins/toolinjector"
PLUGIN_SO="$PLUGIN_DIR/build/tool_injector.so"
SERVER_BIN="$REPO_ROOT/tmp/bifrost-http"
APP_DIR="$(mktemp -d /tmp/bifrost-toolinjector-XXXXXX)"
SERVER_LOG="$APP_DIR/bifrost.log"
MOCK_PORT="18081"
MOCK_BASE_URL="http://127.0.0.1:${MOCK_PORT}"
MOCK_LOG="$APP_DIR/mock-upstream.log"
PORT="18080"
BASE_URL="http://127.0.0.1:${PORT}"
SERVER_PID=""
MOCK_PID=""
GO_CACHE="$(mktemp -d /tmp/bifrost-go-cache-XXXXXX)"
GO_MOD_CACHE="$(mktemp -d /tmp/bifrost-go-modcache-XXXXXX)"

cleanup() {
	if [ -n "$MOCK_PID" ] && kill -0 "$MOCK_PID" 2>/dev/null; then
		kill "$MOCK_PID" 2>/dev/null || true
		wait "$MOCK_PID" 2>/dev/null || true
	fi
	if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -rf "$APP_DIR"
}
trap cleanup EXIT

echo "Building tool-injector plugin..."
rm -f "$PLUGIN_SO"
if command -v make >/dev/null 2>&1; then
	(
		cd "$PLUGIN_DIR" && \
			GOCACHE="$GO_CACHE" GOMODCACHE="$GO_MOD_CACHE" make build-test-plugin
	)
else
	mkdir -p "$PLUGIN_DIR/build"
	(
		cd "$PLUGIN_DIR" && \
			GOCACHE="$GO_CACHE" GOMODCACHE="$GO_MOD_CACHE" CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
				-buildmode=plugin \
				-a -trimpath \
				-tags "sqlite_static" \
				-o "$PLUGIN_SO" \
				main.go
	)
fi

if [ ! -f "$PLUGIN_SO" ]; then
	echo "ERROR: tool-injector plugin was not built at $PLUGIN_SO"
	exit 1
fi

echo "Building bifrost-http server binary..."
rm -f "$SERVER_BIN"
(
	cd "$REPO_ROOT/transports/bifrost-http" && \
		GOCACHE="$GO_CACHE" GOMODCACHE="$GO_MOD_CACHE" CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
			-ldflags="-w -s -X main.Version=vdev-build" \
			-a -trimpath \
			-tags "sqlite_static" \
			-o "$SERVER_BIN" \
			.
)

if [ ! -x "$SERVER_BIN" ]; then
	echo "ERROR: bifrost-http binary not found at $SERVER_BIN"
	exit 1
fi

echo "Starting mock upstream on ${MOCK_BASE_URL}..."
python3 - <<'PY' >"$MOCK_LOG" 2>&1 &
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        _ = self.rfile.read(length) if length else b""
        body = {
            "id": "chatcmpl-mock",
            "object": "chat.completion",
            "created": 1710000000,
            "model": "mock-model",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": "mock upstream response"
                    },
                    "finish_reason": "stop"
                }
            ]
        }
        payload = json.dumps(body).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        return


server = ThreadingHTTPServer(("127.0.0.1", 18081), Handler)
server.serve_forever()
PY
MOCK_PID=$!

cat > "$APP_DIR/config.json" <<EOF
{
  "\$schema": "https://www.getbifrost.ai/schema",
  "providers": {
    "openrouter": {
      "keys": [
        {
          "name": "dummy-openrouter-key",
          "value": "sk-dummy",
          "weight": 1,
          "models": ["*"]
        }
      ]
    },
    "ollama": {
      "network_config": {
        "base_url": "$MOCK_BASE_URL"
      }
    }
  },
  "plugins": [
    {
      "name": "tool-injector",
      "path": "$PLUGIN_SO",
      "enabled": true,
      "config": {
        "type": "openrouter:web_search",
        "providers": ["openrouter"],
        "echo_tools": true,
        "parameters": {}
      }
    }
  ]
}
EOF

echo "Starting Bifrost on ${BASE_URL}..."
"$SERVER_BIN" -host 127.0.0.1 -port "$PORT" -app-dir "$APP_DIR" -log-level warn -log-style pretty >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 60); do
	if curl -fsS "${BASE_URL}/health" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

if ! curl -fsS "${BASE_URL}/health" >/dev/null 2>&1; then
	echo "ERROR: Bifrost did not become ready"
	cat "$SERVER_LOG"
	exit 1
fi

echo "Running Newman tests..."
newman run "$API_DIR/collections/tool-injector-test.postman_collection.json" --env-var "base_url=$BASE_URL"

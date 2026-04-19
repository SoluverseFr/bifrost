#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
API_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
REPO_ROOT="$(cd "$API_DIR/../../.." && pwd)"

PLUGIN_DIR="$REPO_ROOT/plugins/openrouterrouting"
PLUGIN_SO="$PLUGIN_DIR/build/openrouter_provider_routing.so"
SERVER_BIN="$REPO_ROOT/tmp/bifrost-http"
APP_DIR="$(mktemp -d /tmp/bifrost-openrouter-routing-XXXXXX)"
SERVER_LOG="$APP_DIR/bifrost.log"
MOCK_PORT="18083"
MOCK_BASE_URL="http://127.0.0.1:${MOCK_PORT}"
MOCK_LOG="$APP_DIR/mock-upstream.log"
PORT="18082"
BASE_URL="http://127.0.0.1:${PORT}"
REQUEST_DIR="$APP_DIR/requests"
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

fail() {
	echo "ERROR: $1"
	if [ -f "$SERVER_LOG" ]; then
		echo
		echo "--- Bifrost log ---"
		cat "$SERVER_LOG"
	fi
	if [ -f "$MOCK_LOG" ]; then
		echo
		echo "--- Mock log ---"
		cat "$MOCK_LOG"
	fi
	exit 1
}

mkdir -p "$REQUEST_DIR"

echo "Building openrouter-provider-routing plugin..."
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
	fail "openrouter-provider-routing plugin was not built at $PLUGIN_SO"
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
	fail "bifrost-http binary not found at $SERVER_BIN"
fi

echo "Starting mock OpenRouter upstream on ${MOCK_BASE_URL}..."
REQUEST_DIR="$REQUEST_DIR" python3 - <<'PY' >"$MOCK_LOG" 2>&1 &
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os

request_dir = os.environ["REQUEST_DIR"]
counter = {"value": 0}


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        counter["value"] += 1
        length = int(self.headers.get("Content-Length", "0"))
        body_bytes = self.rfile.read(length) if length else b""
        body_path = os.path.join(request_dir, f"request-{counter['value']}.json")
        with open(body_path, "wb") as fh:
            fh.write(body_bytes)

        if self.path.endswith("/responses"):
            body = {
                "id": f"resp-mock-{counter['value']}",
                "object": "response",
                "created_at": 1710000000,
                "model": "mock-model",
                "status": "completed",
                "output": [
                    {
                        "id": f"msg-mock-{counter['value']}",
                        "type": "message",
                        "status": "completed",
                        "role": "assistant",
                        "content": [
                            {
                                "type": "output_text",
                                "text": f"mock responses output {counter['value']}",
                                "annotations": []
                            }
                        ]
                    }
                ],
                "tools": [],
                "parallel_tool_calls": True,
                "text": {"format": {"type": "text"}},
                "usage": {
                    "input_tokens": 1,
                    "output_tokens": 1,
                    "total_tokens": 2
                }
            }
        else:
            body = {
                "id": f"chatcmpl-mock-{counter['value']}",
                "object": "chat.completion",
                "created": 1710000000,
                "model": "mock-model",
                "choices": [
                    {
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "content": f"mock upstream response {counter['value']}"
                        },
                        "finish_reason": "stop"
                    }
                ],
                "usage": {
                    "prompt_tokens": 1,
                    "completion_tokens": 1,
                    "total_tokens": 2
                }
            }
        payload = json.dumps(body).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        return


server = ThreadingHTTPServer(("127.0.0.1", 18083), Handler)
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
      ],
      "network_config": {
        "base_url": "$MOCK_BASE_URL"
      }
    }
  },
  "plugins": [
    {
      "name": "openrouter-provider-routing",
      "path": "$PLUGIN_SO",
      "enabled": true,
      "config": {
        "providers": ["openrouter"],
        "rules": [
          {
            "models": ["openai/gpt-5*"],
            "provider": {
              "only": ["azure"],
              "allow_fallbacks": false,
              "require_parameters": true
            }
          }
        ]
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
	fail "Bifrost did not become ready"
fi

echo "Sending matching OpenRouter request..."
MATCH_RESPONSE="$APP_DIR/matching-response.json"
curl -fsS "${BASE_URL}/v1/chat/completions" \
	-H "Content-Type: application/json" \
	-d '{
	  "model": "openrouter/openai/gpt-5-mini",
	  "messages": [{"role": "user", "content": "hello"}]
	}' >"$MATCH_RESPONSE"

echo "Sending non-matching OpenRouter request..."
NO_MATCH_RESPONSE="$APP_DIR/non-matching-response.json"
curl -fsS "${BASE_URL}/v1/chat/completions" \
	-H "Content-Type: application/json" \
	-d '{
	  "model": "openrouter/anthropic/claude-sonnet-4",
	  "messages": [{"role": "user", "content": "hello"}]
	}' >"$NO_MATCH_RESPONSE"

echo "Sending matching OpenRouter responses request..."
MATCH_RESPONSES_RESPONSE="$APP_DIR/matching-responses-response.json"
curl -fsS "${BASE_URL}/v1/responses" \
	-H "Content-Type: application/json" \
	-d '{
	  "model": "openrouter/openai/gpt-5-mini",
	  "input": "hello"
	}' >"$MATCH_RESPONSES_RESPONSE"

MATCH_REQUEST="$REQUEST_DIR/request-1.json"
NO_MATCH_REQUEST="$REQUEST_DIR/request-2.json"
MATCH_RESPONSES_REQUEST="$REQUEST_DIR/request-3.json"

[ -f "$MATCH_REQUEST" ] || fail "missing captured upstream request for matching model"
[ -f "$NO_MATCH_REQUEST" ] || fail "missing captured upstream request for non-matching model"
[ -f "$MATCH_RESPONSES_REQUEST" ] || fail "missing captured upstream request for matching responses model"

jq -e '.choices[0].message.content == "mock upstream response 1"' "$MATCH_RESPONSE" >/dev/null || fail "unexpected matching response body"
jq -e '.choices[0].message.content == "mock upstream response 2"' "$NO_MATCH_RESPONSE" >/dev/null || fail "unexpected non-matching response body"
jq -e '.output[0].content[0].text == "mock responses output 3"' "$MATCH_RESPONSES_RESPONSE" >/dev/null || fail "unexpected matching responses body"

jq -e '.provider.only == ["azure"]' "$MATCH_REQUEST" >/dev/null || fail "matching request did not inject provider.only"
jq -e '.provider.allow_fallbacks == false' "$MATCH_REQUEST" >/dev/null || fail "matching request did not inject allow_fallbacks=false"
jq -e '.provider.require_parameters == true' "$MATCH_REQUEST" >/dev/null || fail "matching request did not inject require_parameters=true"

jq -e 'has("provider") | not' "$NO_MATCH_REQUEST" >/dev/null || fail "non-matching request unexpectedly injected provider policy"
jq -e '.provider.only == ["azure"]' "$MATCH_RESPONSES_REQUEST" >/dev/null || fail "matching responses request did not inject provider.only"
jq -e '.provider.allow_fallbacks == false' "$MATCH_RESPONSES_REQUEST" >/dev/null || fail "matching responses request did not inject allow_fallbacks=false"
jq -e '.provider.require_parameters == true' "$MATCH_RESPONSES_REQUEST" >/dev/null || fail "matching responses request did not inject require_parameters=true"

echo "OpenRouter provider routing e2e test passed."

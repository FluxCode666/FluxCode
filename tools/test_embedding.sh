#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
手动验证 OpenAI 兼容的 Embedding 接口。

用法：
  EMBEDDING_BASE_URL="https://example.com" \
  EMBEDDING_API_KEY="sk-..." \
  EMBEDDING_MODEL="Qwen/Qwen3-Embedding-8B" \
  ./tools/test_embedding.sh

可选环境变量：
  EMBEDDING_INPUT     测试文本，默认：FluxCode embedding smoke test
  CURL_CONNECT_TIMEOUT 连接超时秒数，默认：10
  CURL_MAX_TIME        单次请求最大秒数，默认：60

BASE_URL 可以填写站点根地址或以 /v1 结尾的地址。
API Key 通过环境变量传入，避免写入 shell 历史。
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

for command_name in curl python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少依赖：$command_name" >&2
    exit 1
  fi
done

: "${EMBEDDING_BASE_URL:?请设置 EMBEDDING_BASE_URL，例如 https://example.com}"
: "${EMBEDDING_API_KEY:?请设置 EMBEDDING_API_KEY}"

embedding_model="${EMBEDDING_MODEL:-Qwen/Qwen3-Embedding-8B}"
embedding_input="${EMBEDDING_INPUT:-FluxCode embedding smoke test}"
connect_timeout="${CURL_CONNECT_TIMEOUT:-10}"
max_time="${CURL_MAX_TIME:-60}"

api_base="${EMBEDDING_BASE_URL%/}"
if [[ "$api_base" != */v1 ]]; then
  api_base="$api_base/v1"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

pretty_print() {
  local response_file="$1"
  if ! python3 -m json.tool "$response_file"; then
    echo
    echo "原始响应："
    command cat "$response_file"
    echo
  fi
}

request() {
  local method="$1"
  local url="$2"
  local output_file="$3"
  shift 3

  curl --silent --show-error \
    --connect-timeout "$connect_timeout" \
    --max-time "$max_time" \
    --request "$method" \
    --header "Authorization: Bearer $EMBEDDING_API_KEY" \
    --output "$output_file" \
    --write-out '%{http_code}' \
    "$@" \
    "$url"
}

models_response="$tmp_dir/models.json"
echo "[1/2] 获取模型列表：$api_base/models"
models_status="$(request GET "$api_base/models" "$models_response")"
echo "HTTP $models_status"
pretty_print "$models_response"
if [[ "$models_status" != 2* ]]; then
  echo "获取模型列表失败。" >&2
  exit 1
fi

if ! python3 - "$models_response" "$embedding_model" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response_file:
    payload = json.load(response_file)

models = {
    item.get("id")
    for item in payload.get("data", [])
    if isinstance(item, dict)
}
sys.exit(0 if sys.argv[2] in models else 1)
PY
then
  echo "模型列表中未找到：$embedding_model" >&2
  exit 1
fi

request_body="$tmp_dir/request.json"
python3 - "$request_body" "$embedding_model" "$embedding_input" <<'PY'
import json
import sys

with open(sys.argv[1], "w", encoding="utf-8") as request_file:
    json.dump({"model": sys.argv[2], "input": sys.argv[3]}, request_file, ensure_ascii=False)
PY

embedding_response="$tmp_dir/embedding.json"
echo "[2/2] 请求 Embedding：$api_base/embeddings"
embedding_status="$(request POST "$api_base/embeddings" "$embedding_response" \
  --header 'Content-Type: application/json' \
  --data-binary "@$request_body")"
echo "HTTP $embedding_status"
pretty_print "$embedding_response"
if [[ "$embedding_status" != 2* ]]; then
  echo "Embedding 请求失败。" >&2
  exit 1
fi

python3 - "$embedding_response" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response_file:
    payload = json.load(response_file)

data = payload.get("data")
usage = payload.get("usage")
if not isinstance(data, list) or not data:
    raise SystemExit("响应缺少非空 data 数组")
if not isinstance(usage, dict) or not isinstance(usage.get("prompt_tokens"), int):
    raise SystemExit("响应缺少 usage.prompt_tokens")
print(f"验证通过：返回 {len(data)} 条向量，prompt_tokens={usage['prompt_tokens']}")
PY

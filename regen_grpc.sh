#!/bin/bash
set -euo pipefail
export PATH="/opt/protobuf-3.6.1/bin:${PATH}"
protoc \
  --go_out=proto/roxy_v0 \
  --go_opt=paths=source_relative \
  --go-grpc_out=proto/roxy_v0 \
  --go-grpc_opt=paths=source_relative \
  roxy.proto

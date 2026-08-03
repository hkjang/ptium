#!/bin/sh
set -eu

export HTTP_ADDR="${PTIUM_INTERNAL_HTTP_ADDR:-127.0.0.1:8081}"

/app/ptium &
api_pid=$!
nginx -e /dev/stderr -g 'daemon off;' &
web_pid=$!

shutdown() {
    kill -TERM "$api_pid" "$web_pid" 2>/dev/null || true
    wait "$api_pid" "$web_pid" 2>/dev/null || true
}

trap shutdown INT TERM EXIT

while kill -0 "$api_pid" 2>/dev/null && kill -0 "$web_pid" 2>/dev/null; do
    sleep 1
done

if ! kill -0 "$api_pid" 2>/dev/null; then
    wait "$api_pid" || status=$?
else
    wait "$web_pid" || status=$?
fi
exit "${status:-1}"

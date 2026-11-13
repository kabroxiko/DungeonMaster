#!/bin/sh
set -eu

# The API is private to the container; nginx is the only listener operators and reverse proxies talk to. NODE_ENV is
# forced so a production image never starts with the development CORS rules or the insecure development JWT secret.
PORT=5001
DM_BIND_HOST=127.0.0.1
NODE_ENV=production
export PORT DM_BIND_HOST NODE_ENV

/app/gmai-server &
app_pid=$!

nginx -g 'daemon off;' &
nginx_pid=$!

shutdown() {
	kill -TERM "$nginx_pid" "$app_pid" 2>/dev/null || true
}
trap shutdown TERM INT

# Stop the container when either process dies, so a crashed API cannot keep a healthy nginx serving 502s.
while kill -0 "$app_pid" 2>/dev/null && kill -0 "$nginx_pid" 2>/dev/null; do
	sleep 1
done

kill -TERM "$nginx_pid" "$app_pid" 2>/dev/null || true
wait "$nginx_pid" 2>/dev/null || true
wait "$app_pid"

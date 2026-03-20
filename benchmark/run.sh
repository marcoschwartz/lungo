#!/bin/bash
# Lungo vs Next.js vs Bun SSR Benchmark
# All render 100 blog posts dynamically on every request (no caching)

set -e
cd "$(dirname "$0")"

LUNGO_PORT=3501
NEXTJS_PORT=3500
BUN_PORT=3502
REQUESTS=2000
CONCURRENCY=10

echo "============================================"
echo "  Lungo vs Next.js vs Bun SSR Benchmark"
echo "  100 blog posts, dynamic SSR (no cache)"
echo "  $REQUESTS requests, $CONCURRENCY concurrent"
echo "============================================"
echo ""

# Kill any existing processes on these ports
lsof -ti:$LUNGO_PORT 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti:$NEXTJS_PORT 2>/dev/null | xargs kill -9 2>/dev/null || true
lsof -ti:$BUN_PORT 2>/dev/null | xargs kill -9 2>/dev/null || true
sleep 1

# --- Build Time ---
echo "=== Build Time ==="
LUNGO_BUILD_START=$(date +%s%N)
(cd lungo-app && go build -o /tmp/lungo-bench . 2>/dev/null)
LUNGO_BUILD_END=$(date +%s%N)
LUNGO_BUILD_MS=$(( (LUNGO_BUILD_END - LUNGO_BUILD_START) / 1000000 ))
echo "Lungo:   ${LUNGO_BUILD_MS}ms"

NEXTJS_BUILD_START=$(date +%s%N)
(cd nextjs-app && npx next build 2>/dev/null | tail -1)
NEXTJS_BUILD_END=$(date +%s%N)
NEXTJS_BUILD_MS=$(( (NEXTJS_BUILD_END - NEXTJS_BUILD_START) / 1000000 ))
echo "Next.js: ${NEXTJS_BUILD_MS}ms"

echo "Bun:     0ms (no build step)"
echo ""

# --- App Size ---
echo "=== App Size ==="
LUNGO_BIN_SIZE=$(stat -c%s /tmp/lungo-bench 2>/dev/null || echo 0)
NEXTJS_DIR_SIZE=$(du -sb nextjs-app/ 2>/dev/null | cut -f1)
BUN_RUNTIME_SIZE=$(stat -c%s "$(which bun)" 2>/dev/null || echo 0)
BUN_APP_SIZE=$(stat -c%s bun-app/server.jsx 2>/dev/null || echo 0)
BUN_TOTAL=$((BUN_RUNTIME_SIZE + BUN_APP_SIZE))

echo "Lungo:   $(numfmt --to=iec-i $LUNGO_BIN_SIZE 2>/dev/null) (single binary)"
echo "Next.js: $(numfmt --to=iec-i $NEXTJS_DIR_SIZE 2>/dev/null) (node_modules + build)"
echo "Bun:     $(numfmt --to=iec-i $BUN_TOTAL 2>/dev/null) (runtime $(numfmt --to=iec-i $BUN_RUNTIME_SIZE 2>/dev/null) + source $(numfmt --to=iec-i $BUN_APP_SIZE 2>/dev/null))"
echo ""

# Start all servers
echo "Starting servers..."
(cd lungo-app && PORT=$LUNGO_PORT /tmp/lungo-bench) &
LUNGO_PID=$!

(cd nextjs-app && PORT=$NEXTJS_PORT npx next start -p $NEXTJS_PORT > /dev/null 2>&1) &
NEXTJS_PID=$!

(cd bun-app && PORT=$BUN_PORT bun run server.jsx > /dev/null 2>&1) &
BUN_PID=$!

echo "Waiting for servers..."
sleep 5

# Verify all running
for name_port in "Lungo:$LUNGO_PORT" "Next.js:$NEXTJS_PORT" "Bun:$BUN_PORT"; do
    name=${name_port%%:*}
    port=${name_port##*:}
    if ! curl -s -o /dev/null -w "%{http_code}" http://localhost:$port/ | grep -q 200; then
        echo "ERROR: $name not responding on :$port"
    fi
done
echo "All servers running."
echo ""

# --- HTML Response Size ---
LUNGO_SIZE=$(curl -s http://localhost:$LUNGO_PORT/ | wc -c)
NEXTJS_SIZE=$(curl -s http://localhost:$NEXTJS_PORT/ | wc -c)
BUN_SIZE_HTML=$(curl -s http://localhost:$BUN_PORT/ | wc -c)
echo "=== HTML Response Size ==="
echo "Lungo:   $LUNGO_SIZE bytes"
echo "Next.js: $NEXTJS_SIZE bytes"
echo "Bun:     $BUN_SIZE_HTML bytes"
echo ""

# Helper: get total RSS for a framework
# Uses lsof for port-based lookup, falls back to pgrep for Next.js (next-server)
get_mem_kb() {
    local port=$1
    local name=$2
    local total=0
    # Try lsof first
    for pid in $(lsof -ti:$port 2>/dev/null); do
        local mem=$(ps -o rss= -p $pid 2>/dev/null | tr -d ' ')
        [ -n "$mem" ] && total=$((total + mem))
    done
    # Fallback for Next.js: find next-server process via pgrep
    if [ "$total" -eq 0 ] && [ "$name" = "Next.js" ]; then
        for pid in $(pgrep -f "next-server" 2>/dev/null); do
            local mem=$(ps -o rss= -p $pid 2>/dev/null | tr -d ' ')
            [ -n "$mem" ] && total=$((total + mem))
        done
    fi
    echo $total
}

# --- Memory (idle) ---
echo "=== Memory Usage (idle) ==="
for name_port in "Lungo:$LUNGO_PORT" "Next.js:$NEXTJS_PORT" "Bun:$BUN_PORT"; do
    name=${name_port%%:*}
    port=${name_port##*:}
    mem=$(get_mem_kb $port "$name")
    [ "$mem" -gt 0 ] && echo "$name:   ${mem} KB ($(numfmt --to=iec-i $((mem * 1024)) 2>/dev/null || echo ${mem}KB))"
done
echo ""

# Heavy warmup
echo "Warming up (500 requests each)..."
ab -n 500 -c 10 http://localhost:$LUNGO_PORT/ > /dev/null 2>&1
ab -n 500 -c 10 http://localhost:$NEXTJS_PORT/ > /dev/null 2>&1
ab -n 500 -c 10 http://localhost:$BUN_PORT/ > /dev/null 2>&1
echo ""

# --- Memory (warmed) ---
echo "=== Memory Usage (warmed) ==="
for name_port in "Lungo:$LUNGO_PORT" "Next.js:$NEXTJS_PORT" "Bun:$BUN_PORT"; do
    name=${name_port%%:*}
    port=${name_port##*:}
    mem=$(get_mem_kb $port "$name")
    [ "$mem" -gt 0 ] && echo "$name:   ${mem} KB ($(numfmt --to=iec-i $((mem * 1024)) 2>/dev/null || echo ${mem}KB))"
done
echo ""

# --- Throughput ---
for name_port in "Lungo:$LUNGO_PORT" "Next.js:$NEXTJS_PORT" "Bun:$BUN_PORT"; do
    name=${name_port%%:*}
    port=${name_port##*:}
    echo "=== Throughput: $name ($REQUESTS req, $CONCURRENCY concurrent) — 3 rounds ==="
    for i in 1 2 3; do
        echo -n "  Round $i: "
        ab -n $REQUESTS -c $CONCURRENCY -q http://localhost:$port/ 2>/dev/null | grep "Requests per second"
    done
    echo ""
done

# --- Single Request Latency ---
echo "=== Single Request Latency (avg of 20) ==="
for name_port in "Lungo:$LUNGO_PORT" "Next.js:$NEXTJS_PORT" "Bun:$BUN_PORT"; do
    name=${name_port%%:*}
    port=${name_port##*:}
    times=""
    for i in $(seq 1 20); do
        T=$(curl -s -o /dev/null -w "%{time_total}" http://localhost:$port/)
        times="$times $T"
    done
    avg=$(echo $times | tr ' ' '\n' | awk '{s+=$1} END {printf "%.1f", s/NR*1000}')
    echo "$name:   ${avg}ms"
done
echo ""

# --- Memory (final) ---
echo "=== Memory Usage (after benchmark) ==="
for name_port in "Lungo:$LUNGO_PORT" "Next.js:$NEXTJS_PORT" "Bun:$BUN_PORT"; do
    name=${name_port%%:*}
    port=${name_port##*:}
    mem=$(get_mem_kb $port "$name")
    [ "$mem" -gt 0 ] && echo "$name:   ${mem} KB ($(numfmt --to=iec-i $((mem * 1024)) 2>/dev/null || echo ${mem}KB))"
done
echo ""

# Cleanup
echo "Stopping servers..."
kill $LUNGO_PID $NEXTJS_PID $BUN_PID 2>/dev/null
wait $LUNGO_PID $NEXTJS_PID $BUN_PID 2>/dev/null || true
echo "Done."

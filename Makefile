.PHONY: dev build sync-runtime css test

# Sync the runtime JS into the embedded package location
sync-runtime:
	cp runtime/lungo.js pkg/lungo/lungo_runtime.js

# Build Tailwind CSS
css:
	cd _example && npx @tailwindcss/cli -i app/input.css -o static/styles.css

# Build the package
build: sync-runtime
	go build ./pkg/lungo/

# Run tests
test: sync-runtime
	go test ./pkg/lungo/ -v

# Run the example in dev mode
dev: sync-runtime css
	cd _example && LUNGO_DEV=1 go run .

# Build the example binary
bin: sync-runtime css
	go build -o bin/lungo-example ./_example/

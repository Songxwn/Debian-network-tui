.PHONY: build test install clean cross

APP := debian-network-tui
LDFLAGS := -s -w

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(APP) .

test:
	go test ./...

install: build
	install -d $(DESTDIR)/usr/local/bin
	install -m 755 bin/$(APP) $(DESTDIR)/usr/local/bin/$(APP)

# Cross-compile for Debian amd64 / arm64
cross:
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(APP)-linux-arm64 .

clean:
	rm -rf bin/

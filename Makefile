BINARY := streamy
VERSION ?= dev
COMMIT ?= dev
LDFLAGS := -X github.com/wingitman/streamy/internal/version.Version=$(VERSION) -X github.com/wingitman/streamy/internal/version.Commit=$(COMMIT)
.PHONY: build test install build-all clean
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/streamy
test:
	go test ./...
install:
	./install.sh
build-all:
	mkdir -p releases
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-linux-amd64 ./cmd/streamy
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-linux-arm64 ./cmd/streamy
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-darwin-amd64 ./cmd/streamy
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-darwin-arm64 ./cmd/streamy
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o releases/$(BINARY)-windows-amd64.exe ./cmd/streamy
clean:
	rm -f $(BINARY) $(BINARY).exe

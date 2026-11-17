VERSION ?= 2.0.0
LDFLAGS := -s -w -X github.com/petealeon/tbible/v2/internal/version.Version=$(VERSION)

.PHONY: all build windows test vet race tidy gen verify dist install clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/tbible ./cmd/tbible

windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/tbible.exe ./cmd/tbible

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

tidy:
	go mod tidy

gen:
	go run ./cmd/tbgen

verify:
	go run ./cmd/tbgen --verify

# Build release archives and a checksum manifest under dist/.
dist:
	bash scripts/dist.sh $(VERSION)

install: build
	./install.sh --local

clean:
	rm -rf bin dist

.PHONY: all build test test-offline bench bench-profile pprof-web clean

BINARY=music-downloader

all: build test

build:
	go build -o $(BINARY) ./cmd/music-downloader

test:
	go test -v ./...

test-offline:
	go test -v -short ./...

bench:
	go test -bench=. -benchmem -run=^$$ ./...

bench-profile:
	go test -bench=BenchmarkDecrypt_10MB -cpuprofile=cpu.prof -memprofile=mem.prof ./deezer
	@echo "\n=== Top CPU Consuming Functions ==="
	go tool pprof -top cpu.prof

pprof-web:
	@if [ ! -f cpu.prof ]; then \
		echo "cpu.prof not found. Run 'make bench-profile' or run binary with -cpuprofile=cpu.prof first."; \
		exit 1; \
	fi
	go tool pprof -http=:8080 cpu.prof

clean:
	rm -f $(BINARY) *.prof *.out *.test

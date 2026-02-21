.PHONY: build test test-verbose test-run test-race vet lint clean install

BINARY     := cobra-shell
BUILD_DIR  := bin
CMD        := ./cmd/cobra-shell

build:
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD)

test:
	go test ./...

test-verbose:
	go test -v ./...

test-run:
	go test -v -run $(RUN) ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)

install:
	go install $(CMD)

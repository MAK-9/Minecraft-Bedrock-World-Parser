export CGO_ENABLED := 0

BINARY := minecraft-bedrock-parser

.PHONY: build install test fixtures vet

build:
	go build -o $(BINARY) ./cmd/parser

install:
	go install ./cmd/parser

fixtures:
	go run testdata/generate_fixtures.go

test: fixtures
	go test ./... -v -count=1

vet:
	go vet ./...

check: vet test

clean:
	rm -f $(BINARY)

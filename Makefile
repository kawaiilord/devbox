.PHONY: test build server client linux

test:
	cd server && go test ./...
	cd client && flutter test

build: server client

server:
	mkdir -p bin
	cd server && go build -o ../bin/sameframe ./cmd/sameframe

client:
	cd client && flutter build web --release

linux:
	cd client && flutter build linux --release

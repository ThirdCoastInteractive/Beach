.PHONY: all test test-integration examples league chessbots chesstui vet build clean

all: vet test build

build:
	go build ./...

vet:
	go vet ./...

test:
	go test -cover ./...

test-integration:
	go test -v -count=1 -cover \
		./eventsource/pg/... \
		./eventsource/clickhouse/...

examples:
	@for dir in $(sort $(dir $(wildcard examples/*/main.go))); do \
		name=$$(basename $$dir); \
		case $$name in chesstui) continue ;; esac; \
		printf "\n=== $$name ===\n"; \
		go run ./$$dir || exit 1; \
	done

league:
	go run ./examples/league

chessbots:
	go run ./examples/chessbots

chesstui:
	go run ./examples/chesstui

clean:
	rm -f *.out

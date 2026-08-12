.PHONY: build test doctor list bench report clean

build:
	go build -o bin/lme ./cmd/lme

test:
	go test ./...

doctor: build
	./bin/lme doctor

list: build
	./bin/lme list

bench: build
	./bin/lme run

report: build
	./bin/lme report

clean:
	rm -rf bin results/results.jsonl

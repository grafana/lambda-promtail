all: clean test build

GOTEST ?= go test

build:
	GOOS=linux CGO_ENABLED=0 go build -o ./main ./...

test:
	$(GOTEST) ./...

clean:
	rm -f main

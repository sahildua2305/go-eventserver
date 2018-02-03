build:
	go build .

run:
	go build .
	./go-eventserver

test:
	go test ./...

test-cover:
	go test -cover ./...

test-race:
	go test -race ./...

test-bench:
	go test -run=XxX -bench=.

fmt:
	gofmt -d $$(gofmt -l ./)

fmt-wet:
	gofmt -w $$(gofmt -l ./)

lint:
	golint ./...

clean:
	rm go-eventserver

harness:
	./extras/followermaze.sh

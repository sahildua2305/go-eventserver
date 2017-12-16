build:
	go build .

run:
	go build .
	./go-eventserver

test:
	go test ./...

test-cover:
	go test -cover ./...

test-bench:
	go test -run=XxX -bench=.

fmt:
	gofmt -d $$(gofmt -l ./)

fmt-wet:
	gofmt -w $$(gofmt -l ./)

clean:
	rm go-eventserver

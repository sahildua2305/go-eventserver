build:
	go build .
run:
	go build .
	./go-eventserver
test:
	go test -v ./...
test-cover:
	go test -cover ./...
test-bench:
	go test -run=XxX -v -bench=.
fmt-dry:
	gofmt -d $$(gofmt -l ./)
fmt:
	gofmt -w $$(gofmt -l ./)
clean:
	rm go-eventserver

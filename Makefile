.PHONY: test coverage coverage-html

test:
	go test ./tests

coverage:
	go test -coverpkg=./src/... -coverprofile=coverage.out ./tests
	go tool cover -func=coverage.out

coverage-html:
	go test -coverpkg=./src/... -coverprofile=coverage.out ./tests
	go tool cover -html=coverage.out -o coverage.html

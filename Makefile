SHELL := /bin/bash
.POSIX:
.PHONY: all clean \
	delete deploy sam-build \
	coverage test medium-tests small-tests static-checks\
	build-Function

# https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-build.html#examples-makefile-identifier
# https://docs.aws.amazon.com/lambda/latest/dg/golang-package.html
# https://github.com/aws-samples/sessions-with-aws-sam/tree/master/go-al2
# https://aws.amazon.com/blogs/compute/migrating-aws-lambda-functions-from-the-go1-x-runtime-to-the-custom-runtime-on-amazon-linux-2/
build-Function:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" \
		-tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap lambda/main.go

static-checks:
	go vet -tags=all_tests ./...
	go run honnef.co/go/tools/cmd/staticcheck -tags=all_tests ./...
	go build -tags=all_tests ./...

small-tests:
	go test -tags=small_tests ./...

medium-tests:
	go test -tags=medium_tests -count=1 ./...

test: static-checks small-tests medium-tests

coverage:
	go test -covermode=count -coverprofile=coverage.out \
	  -tags=small_tests,contract_tests ./...
	go tool cover -html=coverage.out	

sam-build: template.yaml
	sam validate
	sam validate --lint
	sam build

deploy: sam-build deploy.env
	bin/sam-with-env.sh deploy.env deploy

delete: template.yml
	sam delete

clean:
	rm -rf coverage.out .aws-sam
	go clean
	go clean -testcache

all: sam-build

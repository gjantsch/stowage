## -------------------------
## Stowage Makefile
## Usage:
##     make <option>
## -------------------------
help:	## Show this help.
	@sed -ne '/@sed/!s/## //p' $(MAKEFILE_LIST)

build:	## Build the project.
	go build ./... -o stowage

test:	## Test the project.
	go test ./...

lint:	## Lint the project.
	go vet ./...

fmt:	## Format the project.
	go fmt ./...

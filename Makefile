# Set the shell to bash always
SHELL := /bin/bash


############# my stuff
export IS_DEV = true

sqlite-db:
	sqlite3 doc.db < schema/crds_up.sql


build-doc:
	CGO_ENABLED=1 GOOS=linux go build -o doc -mod=readonly -v ./cmd/doc/main.go

build-gitter:
	CGO_ENABLED=1 GOOS=linux go build -o gitter -mod=readonly -v ./cmd/gitter/main.go

run-doc:
	./doc

run-gitter:
	./gitter

copy-static-files:
	cp -r static out/static

serve:
	python -m http.server --directory out

# Run this if you wanna see how it works
all: build-gitter build-doc sqlite-db run-gitter run-doc copy-static-files
# Set the shell to bash always
SHELL := /bin/bash

sqlite-db:
	sqlite3 doc.db < schema/crds_up.sql


doc:
	CGO_ENABLED=1 GOOS=linux go build -o doc -mod=readonly -v ./cmd/doc/main.go

gitter:
	CGO_ENABLED=1 GOOS=linux go build -o gitter -mod=readonly -v ./cmd/gitter/main.go

serve:
	python -m http.server --directory out

all: doc gitter
# Set the shell to bash always
SHELL := /bin/bash

all: doc gitter

# use this to manually initialize a doc.db file with the correct schema.
sqlite-db:
	sqlite3 doc.db < schema/crds_up.sql

# Note: CGO_ENABLED is required for the SQLite3 module.

doc:
	CGO_ENABLED=1 GOOS=linux go build -o doc -mod=readonly -v ./cmd/doc/main.go

gitter:
	CGO_ENABLED=1 GOOS=linux go build -o gitter -mod=readonly -v ./cmd/gitter/main.go

clean:
	rm doc
	rm gitter

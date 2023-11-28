# Set the shell to bash always
SHELL := /bin/bash

# Note: CGO_ENABLED is required for the SQLite3 module.
export CGO_ENABLED=1
export GOOS=linux

all: doc gitter

doc:
	cd docs-generator; go build -o ../doc -mod=readonly ./doc/doc.go

gitter:
	cd docs-generator; go build -o ../gitter -mod=readonly ./gitter/gitter.go

clean:
	rm doc
	rm gitter

# use this to manually initialize a doc.db file with the correct schema.
sqlite-db:
	sqlite3 doc.db < schema/crds_up.sql

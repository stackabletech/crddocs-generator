# Set the shell to bash always
SHELL := /bin/bash


# build-doc:
# 	docker build . -f deploy/doc.Dockerfile -t crdsdev/doc:latest

# build-gitter:
# 	docker build . -f deploy/gitter.Dockerfile -t crdsdev/doc-gitter:latest

# .PHONY: build-doc build-gitter


############# my stuff
export IS_DEV = true
export PG_USER = postgres
export PG_PASSWORD = password
export PG_HOST = localhost
export PG_PORT = 5432
export PG_DB = doc

sqlite-db:
	sqlite3 doc.db < schema/crds_up.sql

run-postgres:
	docker run -d --rm \
	--name dev-postgres \
	-e POSTGRES_PASSWORD=password \
	-p 5432:5432 postgres

build-doc:
	CGO_ENABLED=1 GOOS=linux go build -o doc -mod=readonly -v ./cmd/doc/main.go

build-gitter:
	CGO_ENABLED=1 GOOS=linux go build -o gitter -mod=readonly -v ./cmd/gitter/main.go

run-doc:
	./doc

run-gitter:
	./gitter

# README:
# build both doc and gitter. Start PG. Then in two different terminals start doc and gitter.
# then go to localhost:5000
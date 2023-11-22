#!/usr/bin/env bash
set -euo pipefail

# To run this script you need a config file with your repositories,
# a directory with an HTML template, and a directory of static files to be
# copied over.
#
# You also need go installed. You don't need to pre-build the binaries, 
# but you can do so with `make all`.
#
# args:
# - repos.yaml config file
# - template dir
# - static dir
# - output dir

CONFIG_FILE="$1"
TEMPLATE_DIR="$2"
STATIC_DIR="$3"
OUT_DIR="$4"

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

DB_FILE=$(mktemp --suffix=".db")
trap 'rm -f $DB_FILE' EXIT

echo "Initializing database file '$DB_FILE' ..."
sqlite3 "$DB_FILE" < "$SCRIPT_DIR"/schema/crds_up.sql

echo "Building 'gitter' and 'doc' binaries ..."
make -C "$SCRIPT_DIR" gitter
make -C "$SCRIPT_DIR" doc

echo "Indexing repos defined in '$CONFIG_FILE' ..."
"$SCRIPT_DIR"/gitter --db "$DB_FILE" --config "$CONFIG_FILE"

echo "Generating site into '$OUT_DIR' ..."
"$SCRIPT_DIR"/doc  --db "$DB_FILE" --config "$CONFIG_FILE" --template "$TEMPLATE_DIR" --out "$OUT_DIR"

echo "Copying static files over ..."
mkdir "$OUT_DIR"/static
cp -r "$STATIC_DIR"/* "$OUT_DIR"/static

echo "Done!"
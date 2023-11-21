# CRD docs static generator

This is a generator for a static site for CRD documentation.
It is based on [doc.crds.dev](https://github.com/crdsdev/doc).
It is customized for our (Stackable) repositories.

## Generating the site

run `make all`.

This will build the two go files `gitter` and `doc`, initialize a `doc.db` SQLite3 database, run the two tools and copy over static files into the `out` directory. This is the directory containing the final site.

## Demo serve

You can run `make serve` if you have `python` installed, to serve the `out` directory.

## Changing the repos and tags that are generated

The file `repos.yaml` contains the repos that are generated, as well as the tags to index for each repo.
Update this file to change the repos and tags that you want.

Additionally the `template/home.html` file contains a menu of hardcoded repo names. If a repo is added or removed, update this list too.

## TODO

License?
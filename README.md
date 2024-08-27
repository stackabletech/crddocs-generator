# CRD docs static generator

This is a generator for a static site for CRD documentation based on [Daniel Mangums](https://github.com/hasheddan) work on [doc.crds.dev](https://github.com/crdsdev/doc).
Thank you very much!

This fork is customized for our (Stackable) repositories.

## Building

Run

  make

Two binaries will be created: `gitter` and `doc`.

## The tools

### `gitter`

`gitter` takes the config file and crawls the repos and text to find CRDs for which documentation should be generated.
It stores these in an SQLite database.

### `doc`

`doc` then takes the found CRDs and template files, and generates all the HTML for the website based on the template and CRDs in the database.

## Generating docs

Have a look at https://github.com/stackabletech/crddocs for sample usage.
To generate docs you need a yaml configuration file specifying which repos and tags to document.
It should look like this:

  repos:
    airflow-operator:
      - "24.7.0"
      - "nightly"
    druid-operator:
      - "24.7.0"
      - "nightly"
    hbase-operator:
      - "24.7.0"
      - "nightly"

  platformVersions:
    - "24.7.0"
    - "nightly"

You also need a HTML file template and a directory of static files.

Then, use the `build-site.sh` shell script to build your site.
It contains more instructions on the required arguments.

## Implementation notes - differences to the upstream tool

The `gitter` and `doc` binaries are run in the shell and now accept some commandline arguments. 
Repos are not indexed on demand anymore, but are instead configured in a yaml configuration file (see above).
Both tools use an SQLite3 database for the indexed CRDs, so no Postgres database is necessary.
The shell script tying them together generates the database in a temporary file.

The link structure is slightly different.

The template and static files have been moved out of this repository, as they are user-specific.

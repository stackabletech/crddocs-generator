# CRD docs static generator

This is a generator for a static site for CRD documentation based on hasheddans work for [doc.crds.dev](https://github.com/crdsdev/doc).
Thank you very much!

It is customized for our (Stackable) repositories.

## Generating docs

Have a look at https://github.com/stackabletech/crddocs for sample usage.
To generate docs you need a yaml configuration file specifying which repos and tags to document.
It should look like this (organization > repo > tag):

    stackabletech:
    airflow-operator:
        - "23.7.0"
        - "23.4.0"
        - "23.1.0"
    druid-operator:
        - "23.7.0"
        ...
    ...

You also need a HTML file template and a directory of static files.

Then, use the `build-site.sh` shell script to build your site.
It contains more instructions on the required arguments.

## Implementation notes - differences to the upstream tool

The `gitter` and `doc` binaries are simply run in the shell and now accept some commandline arguments. 
Repos are not indexed on demand anymore, but are instead configured in a yaml configuration file (see above).
Both tools use an SQLite3 database for the indexed CRDs, so no Postgres database is necessary.
The shell script tying them together simply generates the database in a temporary file.

The link structure is slightly different.

The template and static files have been moved out of this repository, as they are user-specific.

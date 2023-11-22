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

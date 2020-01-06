cloudbuild
==========

Abstract
--------

`cloudbuild` is a command line tool for [Google Cloud Build](https://cloud.google.com/cloud-build/).
Still work in progress.

Features
--------

* Alternative for `gcloud builds submit` with some limitations:
    * Unsupported options:
        * `--no-source`
        * `--async`
        * `--no-cache`
        * `--disk-size`
        * `--machine-type`
        * `--timeout`
        * `--tag / -t`
* More robust behaviors.
    * Create source archives in the same way with `docker`.
    * Retries operations.

Usage
-----

Using docker is recommended.

### Using service account

```
$ docker run --rm \
    -v $(pwd):/workspace \
    -v /path/to/credentials.json:/credentials.json \
    -e GOOGLE_APPLICATION_CREDENTIALS=/credentials.json \
    ikedam/cloudbuild .
```

### Using user credentials

```
$ docker run -ti --name gcloud-config google/cloud-sdk:alpine gcloud auth application-default login
$ docker run --rm \
    --volumes-from gcloud-config \
    -v "$(pwd)":/workspace \
    ikedam/cloudbuild .
```

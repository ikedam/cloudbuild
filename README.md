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
    * Limited options:
        * `--substitutions`
            * Supports only commas to separate multiple key-value pairs.
            * `--substitution / -s` is provided instead and recommended. It allows be specified multiple times.
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

Diagnose
--------

You can get stack dump with `SIGUSR1` signal without terminating the container:

```
$ docker kill -s USR1 "$(docker ps -q --filter ancestor=ikedam/cloudbuild)"
```

You can get stack dump and terminate the container with `ABRT` signal:

```
$ docker kill -s ABRT "$(docker ps -q --filter ancestor=ikedam/cloudbuild)"
```

You cannot get stack dump with `HUP`, `INT` or `TERM` signals (`cloudbuild` exit silengly for those signals), but you can have `cloudbuild` to print stack dumps also for those signals by passing `--always-dump` option.

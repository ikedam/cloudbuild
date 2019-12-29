cloudbuild
==========

`cloudbuild` is a command line tool for [Google Cloud Build](https://cloud.google.com/cloud-build/).
Still work in progress.

Goal
----

* Replaces `gcloud submit` commands.
* More robust behaviors.
    * Create source archives in the same way with `docker`.
    * Retry tailing logs.
* Add a feature to join a log stream of already running build.


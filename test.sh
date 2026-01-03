#!/bin/sh

# Run tests on the host system as well as in a docker container.
go test && \
    DOCKER_DEFAULT_PLATFORM=linux/amd64 docker build -t serpent-test . && \
    docker run --rm serpent-test

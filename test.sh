#!/bin/sh

# Run tests on the host system as well as in a docker container.
go test && \
    docker build -t serpent-test . && \
    docker run --rm serpent-test

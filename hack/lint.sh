#!/bin/bash

RETVAL=0
GENERATED_FILES="zz_generated.*.go"


if ! which golint &> /dev/null; then
    echo "Downloading golint tool..."
    go get -u golang.org/x/lint/golint
fi

for file in $(find . -path ./vendor -prune -o -type f -name '*.go' -print | grep -E -v "$GENERATED_FILES"); do
    golint -set_exit_status "$file"
    if [[ $? -ne 0 ]]; then
        RETVAL=1
    fi
done

exit $RETVAL

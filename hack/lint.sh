#!/bin/bash

RETVAL=0
GENERATED_FILES="zz_generated.*.go"


cd $GOPATH || fatal "Failed to enter $GOPATH directory"
if ! which golint &> /dev/null; then
    echo "Downloading golint tool..."
    go install golang.org/x/lint/golint@latest
fi

for file in $(find . -path ./vendor -prune -o -type f -name '*.go' -print | grep -E -v "$GENERATED_FILES"); do
    golint -set_exit_status "$file"
    if [[ $? -ne 0 ]]; then
        RETVAL=1
    fi
done

exit $RETVAL

FROM alpine:3.21
COPY bin/rg-cleanup /usr/local/bin
ENTRYPOINT [ "rg-cleanup" ]

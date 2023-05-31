FROM alpine:3.18
COPY bin/rg-cleanup /usr/local/bin
ENTRYPOINT [ "rg-cleanup" ]

FROM alpine:3.10.3
COPY bin/rg-cleanup /usr/local/bin
ENTRYPOINT [ "rg-cleanup" ]

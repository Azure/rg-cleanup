FROM gcr.io/distroless/static
COPY bin/rg-cleanup /usr/local/bin
ENTRYPOINT [ "rg-cleanup" ]

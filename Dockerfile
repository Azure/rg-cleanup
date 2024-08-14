FROM mcr.microsoft.com/cbl-mariner/base/core:2.0
RUN tdnf install -y azure-cli jq && tdnf clean all
COPY rg-cleanup.sh /usr/local/bin
COPY bin/rg-cleanup ./bin
ENTRYPOINT [ "rg-cleanup.sh" ]

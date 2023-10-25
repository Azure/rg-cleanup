# rg-cleanup
A tool that bulk removes stale resource groups in an Azure subscription.

## Usage

### Prerequisites

- Service principal credentials or User Assigned Managed Identity (CLIENT_ID)
- An Azure subscription

```bash
export AAD_CLIENT_ID=...
export SUBSCRIPTION_ID=...
export AAD_CLIENT_SECRET=...
export TENANT_ID=...
make
./bin/rg-cleanup
```

Use `--identity` to use UAMI

```bash
export AAD_CLIENT_ID="<CLIENT_ID>"
export SUBSCRIPTION_ID="<SUBSCRIPTION_ID>"
make
./bin/rg-cleanup --identity
```

By default, this tool deletes stale resource groups that are older than three days. If you want to customize that, you could add a flag `--ttl=...` when running. For example, if you want to delete stale resource groups that are older than one day, add `--ttl=1d`.

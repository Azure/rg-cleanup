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

For regex support use `--regex "<string-regex-pattern>"`. This flag will look into fully matching regex with the resource group name, meaning a partial regex pattern will not match:
RG Name `kubetest-123` if we have regex pattern `kube` this will not match. A matching pattern will look like `^kube.+$`, `kube.+$`, `^kube.+`, etc.

A deployment bicep file for a logic app running rg-cleanup is available under [templates](./templates):
The following example deployment command assumes:
1. You already set up a user-managed identity (UAMI) and a resource group.
2. You have available the rg-cleanup image in a registry.
The resources will get deployed to the same resource group as the UAMI.
```sh
az deployment group create -g "<rg-name>" -f ./templates/rg-cleaner-logic-app-uami.bicep --parameter \
    uami="<UAMI Name>" \ # Required
    uami_client_id="<UAMI Client ID>" \ # Required
    image="<rg-cleanup image url>" \ # Required
    dryrun="(false|true)" \ # Optional (default: true)
    ttl="<ttl>" \ # Optional
    regex="<regex expression patter>" # Optional
# Other optional parameters are available, please refer to the deployment bicep file.
```

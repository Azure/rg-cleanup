#!/bin/bash

set -euo pipefail

# Parse out --role-assignments flag
role_assignments_flag=false
while [[ "$#" -gt 0 ]]; do
  case $1 in
    --role-assignments) role_assignments_flag=true ;;
    *) args+=("$1") ;;  # Store other arguments in an array
  esac
  shift
done

# Call resource group cleanup binary with its command-line arguments
./bin/rg-cleanup "${args[@]}"

# Exit if not cleaning up unattached role assignments
if [ "$role_assignments_flag" != true ]; then
  echo "Skipping unattached role assignment cleanup..."
  exit 0
fi

# Clean up unattached role assignments
az login --identity
az account set --subscription "${SUBSCRIPTION_ID}"

assignments=$(az role assignment list --scope "/subscriptions/$SUBSCRIPTION_ID" -o tsv --query "[?principalName==''].id")
if [ -z "$assignments" ]; then
  echo "No unattached role assignments found."
  exit 0
fi
echo "Deleting unattached role assignments:"
echo "$assignments"
xargs az role assignment delete --ids <<< "$assignments"

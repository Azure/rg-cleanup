package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

const (
	defaultTTL            = 3 * 24 * time.Hour
	creationTimestampTag  = "creationTimestamp"
	doNotDeleteTag        = "DO-NOT-DELETE"
	aadClientIDEnvVar     = "AAD_CLIENT_ID"
	aadClientSecretEnvVar = "AAD_CLIENT_SECRET"
	tenantIDEnvVar        = "TENANT_ID"
	subscriptionIDEnvVar  = "SUBSCRIPTION_ID"
)

var rfc3339Layouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	// The following two layouts are also acceptable
	// RFC3339 layouts. See:
	// https://github.com/golang/go/issues/20555#issuecomment-440348440
	"2006-01-02T15:04:05+0000",
	"2006-01-02T15:04:05-0000",
	"2006-01-02T15:04:05-00:00",
	"2006-01-02T15:04:05+00:00",
}

type options struct {
	clientID       string
	clientSecret   string
	tenantID       string
	subscriptionID string
	dryRun         bool
	ttl            time.Duration
	identity       bool
}

func (o *options) validate() error {
	if o.clientID == "" {
		return fmt.Errorf("$%s is empty", aadClientIDEnvVar)
	}
	if o.subscriptionID == "" {
		return fmt.Errorf("$%s is empty", subscriptionIDEnvVar)
	}
	if o.identity {
		return nil
	}
	if o.clientSecret == "" {
		return fmt.Errorf("$%s is empty", aadClientSecretEnvVar)
	}
	if o.tenantID == "" {
		return fmt.Errorf("$%s is empty", tenantIDEnvVar)
	}
	return nil
}

func defineOptions() *options {
	o := options{}
	o.clientID = os.Getenv(aadClientIDEnvVar)
	o.clientSecret = os.Getenv(aadClientSecretEnvVar)
	o.tenantID = os.Getenv(tenantIDEnvVar)
	o.subscriptionID = os.Getenv(subscriptionIDEnvVar)
	flag.BoolVar(&o.dryRun, "dry-run", false, "Set to true if we should run the cleanup tool without deleting the resource groups.")
	flag.BoolVar(&o.identity, "identity", false, "Set to true if we should user-assigned identity for AUTH")
	flag.DurationVar(&o.ttl, "ttl", defaultTTL, "The duration we allow resource groups to live before we consider them to be stale.")
	flag.Parse()
	return &o
}

func main() {
	log.Println("Initializing rg-cleanup")

	o := defineOptions()
	if err := o.validate(); err != nil {
		log.Fatalf("Error when validating options: %v", err)
	}

	if o.dryRun {
		log.Println("Dry-run enabled - printing logs but not actually deleting resource groups")
	}

	r, err := getResourceGroupClient(o.clientID, o.clientSecret, o.tenantID, o.subscriptionID, o.identity)
	if err != nil {
		log.Fatalf("Error when obtaining resource group client: %v", err)
	}

	if err := run(context.Background(), r, o.ttl, o.dryRun); err != nil {
		log.Fatalf("Error when running rg-cleanup: %v", err)
	}
}

func run(ctx context.Context, r *armresources.ResourceGroupsClient, ttl time.Duration, dryRun bool) error {
	log.Println("Scanning for stale resource groups")

	pager := r.NewListPager(nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("error when iterating resource groups: %v", err)
		}
		for _, rg := range nextResult.Value {
			rgName := *rg.Name
			if age, ok := shouldDeleteResourceGroup(rg, ttl); ok {
				if dryRun {
					log.Printf("Dry-run: skip deletion of eligible resource group '%s' (age: %s)", rgName, age)
					continue
				}

				// Start the delete without waiting for it to complete.
				log.Printf("Beginning to delete resource group '%s' (age: %s)", rgName, age)
				_, err = r.BeginDelete(ctx, rgName, nil)
				if err != nil {
					log.Printf("Error when deleting %s: %v", rgName, err)
				}
			}
		}
	}

	return nil
}

func shouldDeleteResourceGroup(rg *armresources.ResourceGroup, ttl time.Duration) (string, bool) {
	if _, ok := rg.Tags[doNotDeleteTag]; ok {
		return "", false
	}

	creationTimestamp, ok := rg.Tags[creationTimestampTag]
	if !ok {
		return fmt.Sprintf("probably a long time because it does not have a '%s' tag. Found tags: %v", creationTimestampTag, rg.Tags), true
	}

	var t time.Time
	var err error
	for _, layout := range rfc3339Layouts {
		t, err = time.Parse(layout, *creationTimestamp)
		if err == nil {
			break
		}
	}

	if err != nil {
		log.Printf("failed to parse timestamp: %s", err)
		return "", false
	}

	return fmt.Sprintf("%d days", int(time.Since(t).Hours()/24)), time.Since(t) >= ttl
}

func getResourceGroupClient(clientID, clientSecret, tenantID, subscriptionID string, identity bool) (*armresources.ResourceGroupsClient, error) {
	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.AzurePublic,
		},
	}
	possibleTokens := []azcore.TokenCredential{}
	if identity {
		micOptions := azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(clientID),
		}
		miCred, err := azidentity.NewManagedIdentityCredential(&micOptions)
		if err != nil {
			return nil, err
		}
		possibleTokens = append(possibleTokens, miCred)
	} else {
		spCred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
		if err != nil {
			return nil, err
		}
		possibleTokens = append(possibleTokens, spCred)
	}
	chain, err := azidentity.NewChainedTokenCredential(possibleTokens, nil)
	if err != nil {
		return nil, err
	}
	resourceGroupClient, err := armresources.NewResourceGroupsClient(subscriptionID, chain, &options)
	if err != nil {
		return nil, err
	}
	return resourceGroupClient, nil
}

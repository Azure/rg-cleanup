package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

const (
	defaultTTL            = 3 * 24 * time.Hour
	defaultRegex          = ""
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
	regex          string
	cli            bool
}

func (o *options) validate() error {
	if o.subscriptionID == "" {
		return fmt.Errorf("$%s is empty", subscriptionIDEnvVar)
	}
	if o.cli {
		return nil
	}
	if o.clientID == "" {
		return fmt.Errorf("$%s is empty", aadClientIDEnvVar)
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
	flag.BoolVar(&o.cli, "az-cli", false, "Set to true if we should use az cli for AUTH")
	flag.DurationVar(&o.ttl, "ttl", defaultTTL, "The duration we allow resource groups to live before we consider them to be stale.")
	flag.StringVar(&o.regex, "regex", defaultRegex, "Only delete resource groups matching regex")
	flag.Parse()
	return &o
}

func main() {
	log.Println("Initializing rg-cleanup")
	log.Printf("args: %v\n", os.Args)

	o := defineOptions()
	if err := o.validate(); err != nil {
		log.Printf("Error when validating options: %v", err)
		panic(err)
	}

	if o.dryRun {
		log.Println("Dry-run enabled - printing logs but not actually deleting resource groups")
	}

	r, err := getResourceGroupClient(*o)
	if err != nil {
		log.Printf("Error when obtaining resource group client: %v", err)
		panic(err)
	}

	if err := run(context.Background(), r, o.ttl, o.dryRun, o.regex); err != nil {
		log.Printf("Error when running rg-cleanup: %v", err)
		panic(err)
	}
}

func run(ctx context.Context, r *armresources.ResourceGroupsClient, ttl time.Duration, dryRun bool, regex string) error {
	log.Println("Scanning for stale resource groups")

	pager := r.NewListPager(nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("error when iterating resource groups: %v", err)
		}
		for _, rg := range nextResult.Value {
			rgName := *rg.Name
			if age, ok := shouldDeleteResourceGroup(rg, ttl, regex); ok {
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

func shouldDeleteResourceGroup(rg *armresources.ResourceGroup, ttl time.Duration, regex string) (string, bool) {
	if _, ok := rg.Tags[doNotDeleteTag]; ok {
		return "", false
	}

	if regex != "" {
		match, err := regexMatchesResourceGroupName(regex, *rg.Name)
		if err != nil {
			log.Printf("failed to regex Resource Group Name: %s", err)
			return "", false
		}
		if !match {
			log.Printf("RG '%s' did not match regex", *rg.Name)
			return "", false
		}
		log.Printf("RG '%s' matched regex '%s'", *rg.Name, regex)
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

	return fmt.Sprintf("%d days (%d hours)", int(time.Since(t).Hours()/24), int(time.Since(t).Hours())), time.Since(t) >= ttl
}

func regexMatchesResourceGroupName(regex string, rgName string) (bool, error) {
	if regex != "" {
		rgx, err := regexp.Compile(regex)
		if err != nil {
			return false, fmt.Errorf("failed to compile regex: %v", err)
		}
		match := rgx.FindString(rgName)
		if match != rgName {
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

func getResourceGroupClient(o options) (*armresources.ResourceGroupsClient, error) {
	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.AzurePublic,
		},
	}
	possibleTokens := []azcore.TokenCredential{}
	if o.identity {
		micOptions := azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(o.clientID),
		}
		miCred, err := azidentity.NewManagedIdentityCredential(&micOptions)
		if err != nil {
			return nil, err
		}
		possibleTokens = append(possibleTokens, miCred)
	} else if o.clientSecret != "" {
		spCred, err := azidentity.NewClientSecretCredential(o.tenantID, o.clientID, o.clientSecret, nil)
		if err != nil {
			return nil, err
		}
		possibleTokens = append(possibleTokens, spCred)
	} else if o.cli {
		cliCred, err := azidentity.NewAzureCLICredential(nil)
		if err != nil {
			return nil, err
		}
		possibleTokens = append(possibleTokens, cliCred)
	} else {
		log.Println("unknown login option. login may not succeed")
	}
	chain, err := azidentity.NewChainedTokenCredential(possibleTokens, nil)
	if err != nil {
		return nil, err
	}
	resourceGroupClient, err := armresources.NewResourceGroupsClient(o.subscriptionID, chain, &options)
	if err != nil {
		return nil, err
	}
	return resourceGroupClient, nil
}

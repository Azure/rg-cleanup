package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
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
}

func (o *options) validate() error {
	if o.clientID == "" {
		return fmt.Errorf("$%s is empty", aadClientIDEnvVar)
	}
	if o.clientSecret == "" {
		return fmt.Errorf("$%s is empty", aadClientSecretEnvVar)
	}
	if o.tenantID == "" {
		return fmt.Errorf("$%s is empty", tenantIDEnvVar)
	}
	if o.subscriptionID == "" {
		return fmt.Errorf("$%s is empty", subscriptionIDEnvVar)
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

	r, err := getResourceGroupClient(azure.PublicCloud, o.clientID, o.clientSecret, o.tenantID, o.subscriptionID)
	if err != nil {
		log.Fatalf("Error when obtaining resource group client: %v", err)
	}

	if err := run(context.Background(), r, o.ttl, o.dryRun); err != nil {
		log.Fatalf("Error when running rg-cleanup: %v", err)
	}
}

func run(ctx context.Context, r *resources.GroupsClient, ttl time.Duration, dryRun bool) error {
	log.Println("Scanning for stale resource groups")
	for list, err := r.ListComplete(ctx, "", nil); list.NotDone(); err = list.Next() {
		if err != nil {
			return fmt.Errorf("Error when listing all resource groups: %v", err)
		}

		rg := list.Value()
		rgName := *rg.Name
		if age, ok := shouldDeleteResourceGroup(rg, ttl); ok {
			// Does not wait when deleting a resource group
			log.Printf("Deleting resource group '%s' (age: %s)", rgName, age)
			if dryRun {
				continue
			}

			_, err = r.Delete(ctx, rgName)
			if err != nil {
				log.Printf("Error when deleting %s: %v", rgName, err)
			}
		}
	}

	return nil
}

func shouldDeleteResourceGroup(rg resources.Group, ttl time.Duration) (string, bool) {
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

func getResourceGroupClient(env azure.Environment, clientID, clientSecret, tenantID, subscriptionID string) (*resources.GroupsClient, error) {
	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, tenantID)
	if err != nil {
		return nil, err
	}

	armSpt, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.ServiceManagementEndpoint)
	if err != nil {
		return nil, err
	}

	r := resources.NewGroupsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID)
	r.Authorizer = autorest.NewBearerAuthorizer(armSpt)
	return &r, err
}

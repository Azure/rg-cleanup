package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
)

const (
	creationTimestampTag = "creationTimestamp"
	defaultTTL           = 3 * 24 * time.Hour
	defaultPeriod        = 24 * time.Hour
)

// Consider resource groups with one of the following prefixes deletable
var deletableResourceGroupPrefixes = []string{
	"kubetest-",
	"azuredisk-csi-driver-",
	"azuredisk-csi-driver-",
}

type options struct {
	clientID       string
	clientSecret   string
	tenantID       string
	subscriptionID string
	ttl            time.Duration
	period         time.Duration
}

func (o *options) validateFlags() error {
	if o.clientID == "" {
		return fmt.Errorf("--client-id is empty")
	}
	if o.clientSecret == "" {
		return fmt.Errorf("--client-secret is empty")
	}
	if o.tenantID == "" {
		return fmt.Errorf("--tenant-id is empty")
	}
	if o.subscriptionID == "" {
		return fmt.Errorf("--subscription-id is empty")
	}
	return nil
}

func defineFlags() *options {
	o := options{}
	flag.StringVar(&o.clientID, "client-id", "", "The client ID of the service principal.")
	flag.StringVar(&o.clientSecret, "client-secret", "", "The client secret of the service principal.")
	flag.StringVar(&o.tenantID, "tenant-id", "", "The tenant ID of the service principal.")
	flag.StringVar(&o.subscriptionID, "subscription-id", "", "The Azure subscription ID.")
	flag.DurationVar(&o.ttl, "ttl", defaultTTL, "The duration we allow resource groups to live before we consider them to be stale.")
	flag.DurationVar(&o.period, "period", defaultPeriod, "How often we should clean up stale resource groups.")
	flag.Parse()
	return &o
}

func main() {
	log.Println("Initializing rg-cleanup")

	o := defineFlags()
	if err := o.validateFlags(); err != nil {
		log.Fatalf("Error when validating flags: %v", err)
	}

	r, err := getResourceGroupClient(azure.PublicCloud, o.clientID, o.clientSecret, o.tenantID, o.subscriptionID)
	if err != nil {
		log.Fatalf("Error when obtaining resource group client: %v", err)
	}

	ticker := time.NewTicker(o.period)
	ctx := context.Background()
	for ; true; <-ticker.C {
		if err := run(ctx, o.ttl, r); err != nil {
			log.Fatalf("Error when running rg-cleanup: %v", err)
		}
	}
}

func run(ctx context.Context, ttl time.Duration, r *resources.GroupsClient) error {
	log.Println("Scanning for stale resource groups")
	for list, err := r.ListComplete(ctx, "", nil); list.NotDone(); err = list.Next() {
		if err != nil {
			return fmt.Errorf("Error when listing all resource group: %v", err)
		}

		rg := list.Value()
		rgName := *rg.Name
		if age, ok := shouldDeleteResourceGroup(rg, ttl); ok {
			// Does not wait when deleting a resource group
			log.Printf("Deleting resource group '%s' (age: %s)", rgName, age)
			_, err = r.Delete(ctx, rgName)
			if err != nil {
				log.Printf("Error when deleting %s: %v", rgName, err)
				continue
			}
		}
	}

	return nil
}

func shouldDeleteResourceGroup(rg resources.Group, ttl time.Duration) (string, bool) {
	deletable := false
	for _, prefix := range deletableResourceGroupPrefixes {
		if strings.HasPrefix(*rg.Name, prefix) {
			deletable = true
			break
		}
	}
	if !deletable {
		return "", false
	}

	creationTimestamp, ok := rg.Tags[creationTimestampTag]
	if !ok {
		return fmt.Sprintf("probably a long time because it does not have a '%s' tag", creationTimestampTag), true
	}

	t, err := time.Parse(time.RFC3339, *creationTimestamp)
	if err != nil {
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

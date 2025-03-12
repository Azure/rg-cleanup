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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/serviceprincipals"
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
	clientID        string
	clientSecret    string
	tenantID        string
	subscriptionID  string
	dryRun          bool
	ttl             time.Duration
	identity        bool
	regex           string
	cli             bool
	roleAssignments bool
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
	flag.BoolVar(&o.roleAssignments, "role-assignments", false, "Set to true if we should delete role assignments assigned to principals which no longer exist")
	flag.Parse()
	return &o
}

func main() {
	ctx := context.Background()

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

	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.AzurePublic,
		},
	}

	cred, err := getAzureCredential(*o)
	if err != nil {
		log.Printf("Error when obtaining resource group client: %v", err)
		panic(err)
	}

	resourceGroupClient, err := armresources.NewResourceGroupsClient(o.subscriptionID, cred, &options)
	if err != nil {
		log.Printf("Error when obtaining resource group client: %v", err)
		panic(err)
	}

	if err := runResourceGroupCleanup(ctx, resourceGroupClient, o.ttl, o.dryRun, o.regex); err != nil {
		log.Printf("Error when cleaning up resource groups: %v", err)
		panic(err)
	}

	if o.roleAssignments {
		roleAssignmentClient, err := armauthorization.NewRoleAssignmentsClient(o.subscriptionID, cred, &options)
		if err != nil {
			log.Printf("Error when obtaining role assignment client: %v", err)
			panic(err)
		}

		graph, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, nil)
		if err != nil {
			log.Fatal(err)
		}

		if err := runRoleAssignmentCleanup(ctx, o.subscriptionID, roleAssignmentClient, graph, o.dryRun); err != nil {
			log.Printf("Error when cleaning up role assignments: %v", err)
			panic(err)
		}
	} else {
		log.Println("Skipping role assignment cleanup")
	}
}

func runResourceGroupCleanup(ctx context.Context, r *armresources.ResourceGroupsClient, ttl time.Duration, dryRun bool, regex string) error {
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

func getAzureCredential(o options) (*azidentity.ChainedTokenCredential, error) {
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
	return azidentity.NewChainedTokenCredential(possibleTokens, nil)
}

func runRoleAssignmentCleanup(ctx context.Context, subscriptionID string, roleAssignments *armauthorization.RoleAssignmentsClient, graph *msgraphsdk.GraphServiceClient, dryRun bool) error {
	log.Println("Scanning for stale role assignments")

	// Role assignments that might be able to be deleted, by principalID to which they're assigned.
	principalToAssignmentIDs := map[string][]string{}
	filter := "atScope()" // ignore assignments scoped more narrowly than the subscription
	pager := roleAssignments.NewListForSubscriptionPager(&armauthorization.RoleAssignmentsClientListForSubscriptionOptions{
		Filter: &filter,
	})
	for pager.More() {
		assignments, err := pager.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, assignment := range assignments.Value {
			if assignment.Properties.PrincipalType == nil || *assignment.Properties.PrincipalType != armauthorization.PrincipalTypeServicePrincipal {
				continue
			}
			// The atScope() filter doesn't ignore assignments scoped more broadly than the subscription
			if assignment.Properties.Scope == nil || *assignment.Properties.Scope != "/subscriptions/"+subscriptionID {
				continue
			}
			if assignment.Properties.PrincipalID != nil && assignment.ID != nil {
				pid := *assignment.Properties.PrincipalID
				principalToAssignmentIDs[pid] = append(principalToAssignmentIDs[pid], *assignment.ID)
			}
		}
	}
	if len(principalToAssignmentIDs) == 0 {
		log.Println("No role assignments found")
		return nil
	}

	assignedPrincipalIDs := make([]string, 0, len(principalToAssignmentIDs))
	for k := range principalToAssignmentIDs {
		assignedPrincipalIDs = append(assignedPrincipalIDs, k)
	}
	idReq := serviceprincipals.NewGetByIdsPostRequestBody()
	idReq.SetIds(assignedPrincipalIDs)
	idRes, err := graph.ServicePrincipals().GetByIds().PostAsGetByIdsPostResponse(ctx, idReq, &serviceprincipals.GetByIdsRequestBuilderPostRequestConfiguration{})
	if err != nil {
		return fmt.Errorf("error querying graph: %w", err)
	}

	// When a role assignment refers to a principal ID that exists, it should not be deleted.
	for _, id := range idRes.GetValue() {
		if existingID := id.GetId(); existingID != nil {
			delete(principalToAssignmentIDs, *existingID)
		}
	}

	if len(principalToAssignmentIDs) == 0 {
		log.Printf("No unattached role assignments found")
		return nil
	}

	// The remaining assigned principals no longer exist. Role assignments associated with them should be deleted.
	for _, assignments := range principalToAssignmentIDs {
		for _, assignment := range assignments {
			if dryRun {
				log.Printf("Dry-run: skip deletion of eligible role assignment %s", assignment)
				continue
			}
			_, err := roleAssignments.DeleteByID(ctx, assignment, nil)
			if err != nil {
				return fmt.Errorf("failed to delete role assignment %s: %w", assignment, err)
			}
			log.Printf("Deleted role assignment %s", assignment)
		}
	}

	return nil
}

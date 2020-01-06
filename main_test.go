package main

import (
	"testing"
	"time"

	resources "github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest/to"
)

func TestShouldDeleteResourceGroup(t *testing.T) {
	oneDayAgo := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	fourDaysAgo := time.Now().Add(-defaultTTL - 24*time.Hour).Format(time.RFC3339)
	testCases := []struct {
		desc                string
		rg                  resources.Group
		expectedToBeDeleted bool
		expectedAge         string
	}{
		{
			desc:                "deletable resource group that has not lived for more than 3 days",
			rg:                  getResourceGroup("kubetest-123", oneDayAgo),
			expectedToBeDeleted: false,
			expectedAge:         "1 days",
		},
		{
			desc:                "kubetest resource group that lives for more than 3 days",
			rg:                  getResourceGroup("kubetest-456", fourDaysAgo),
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "azuredisk-csi-driver resource group that lives for more than 3 days",
			rg:                  getResourceGroup("azuredisk-csi-driver-456", fourDaysAgo),
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "azurefile-csi-driver resource group that lives for more than 3 days",
			rg:                  getResourceGroup("azurefile-csi-driver-456", fourDaysAgo),
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "blobfuse-csi-driver resource group that lives for more than 3 days",
			rg:                  getResourceGroup("blobfuse-csi-driver-456", fourDaysAgo),
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "non-deletable resource group",
			rg:                  getResourceGroup("resource group", fourDaysAgo),
			expectedToBeDeleted: false,
			expectedAge:         "",
		},
		{
			desc:                "deletable resource group with no creation timestamp",
			rg:                  getResourceGroup("kubetest-789", ""),
			expectedToBeDeleted: true,
			expectedAge:         "probably a long time because it does not have a 'creationTimestamp' tag",
		},
		{
			desc:                "deletable resource group with invalid creation timestamp",
			rg:                  getResourceGroup("kubetest-789", "invalid creation timestamp"),
			expectedToBeDeleted: false,
			expectedAge:         "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			age, ok := shouldDeleteResourceGroup(tc.rg, defaultTTL)
			if ok != tc.expectedToBeDeleted {
				t.Fatalf("expected %t, but got %t", tc.expectedToBeDeleted, ok)
			}
			if age != tc.expectedAge {
				t.Fatalf("expected the resource group age to be '%s', but got '%s'", tc.expectedAge, age)
			}
		})
	}
}

func getResourceGroup(name, creationTimestamp string) resources.Group {
	var tags map[string]*string
	if creationTimestamp != "" {
		tags = map[string]*string{
			creationTimestampTag: to.StringPtr(creationTimestamp),
		}
	}
	return resources.Group{
		Name: to.StringPtr(name),
		Tags: tags,
	}
}

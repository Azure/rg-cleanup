package main

import (
	"testing"
	"time"

	resources "github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest/to"
)

func TestShouldDeleteResourceGroup(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	threeDaysAgo := time.Now().Add(-defaultTTL).Format(time.RFC3339)
	testCases := []struct {
		desc                string
		rg                  resources.Group
		expectedToBeDeleted bool
		expectedAge         string
	}{
		{
			desc:                "deletable resource group that has not lived for more than 3 days",
			rg:                  getResourceGroup("kubetest-123", now),
			expectedToBeDeleted: false,
			expectedAge:         "0 days",
		},
		{
			desc:                "deletable resource group that lives for more than 3 days",
			rg:                  getResourceGroup("kubetest-456", threeDaysAgo),
			expectedToBeDeleted: true,
			expectedAge:         "3 days",
		},
		{
			desc:                "non-deletable resource group",
			rg:                  getResourceGroup("resource group", now),
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

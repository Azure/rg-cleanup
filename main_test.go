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
		desc     string
		rg       resources.Group
		expected bool
	}{
		{
			desc:     "deletable resource group that has not lived for more than 3 days",
			rg:       getResourceGroup("kubetest-123", now),
			expected: false,
		},
		{
			desc:     "deletable resource group that lives for more than 3 days",
			rg:       getResourceGroup("kubetest-456", threeDaysAgo),
			expected: true,
		},
		{
			desc:     "non-deletable resource group",
			rg:       getResourceGroup("resource group", now),
			expected: false,
		},
		{
			desc:     "deletable resource group with no creation timestamp",
			rg:       getResourceGroup("kubetest-789", ""),
			expected: false,
		},
		{
			desc:     "deletable resource group with invalid creation timestamp",
			rg:       getResourceGroup("kubetest-789", "invalid creation timestamp"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, result := shouldDeleteResourceGroup(tc.rg, defaultTTL)
			if result != tc.expected {
				t.Fatalf("expected %t, but got %t", tc.expected, result)
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

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
		rgName              string
		creationTimestamp   string
		hasDoNotDelete      bool
		expectedToBeDeleted bool
		expectedAge         string
	}{
		{
			desc:                "deletable resource group that has not lived for more than 3 days",
			rgName:              "kubetest-123",
			creationTimestamp:   oneDayAgo,
			expectedToBeDeleted: false,
			expectedAge:         "1 days",
		},
		{
			desc:                "kubetest resource group that lives for more than 3 days",
			rgName:              "kubetest-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "azuredisk-csi-driver resource group that lives for more than 3 days",
			rgName:              "azuredisk-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "azurefile-csi-driver resource group that lives for more than 3 days",
			rgName:              "azurefile-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "blobfuse-csi-driver resource group that lives for more than 3 days",
			rgName:              "blob-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "blobfuse-csi-driver resource group that lives for more than 3 days",
			rgName:              "blob-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "flannel resource group that lives for more than 3 days",
			rgName:              "flannel-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "ctrd resource group that lives for more than 3 days",
			rgName:              "ctrd-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "capz resource group that lives for more than 3 days",
			rgName:              "capz-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         "4 days",
		},
		{
			desc:                "non-deletable resource group",
			rgName:              "resource group",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: false,
			expectedAge:         "",
		},
		{
			desc:                "deletable resource group with no creation timestamp",
			rgName:              "kubetest-789",
			creationTimestamp:   "",
			expectedToBeDeleted: true,
			expectedAge:         "probably a long time because it does not have a 'creationTimestamp' tag",
		},
		{
			desc:                "deletable resource group with invalid creation timestamp",
			rgName:              "kubetest-789",
			creationTimestamp:   "invalid creation timestamp",
			expectedToBeDeleted: false,
			expectedAge:         "",
		},
		{
			desc:                "deletable resource group but has a DO-NOT-DELETE tag",
			rgName:              "kubetest-789",
			creationTimestamp:   fourDaysAgo,
			hasDoNotDelete:      true,
			expectedToBeDeleted: false,
			expectedAge:         "",
		},
		{
			desc:                "deletable resource group with unix timestamp",
			rgName:              "pkr-Resource-Group-123",
			creationTimestamp:   "1608166242",
			hasDoNotDelete:      false,
			expectedToBeDeleted: true,
			expectedAge:         "29 days",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			tags := make(map[string]*string)
			if tc.creationTimestamp != "" {
				tags[creationTimestampTag] = to.StringPtr(tc.creationTimestamp)
			}
			if tc.hasDoNotDelete {
				tags[doNotDeleteTag] = to.StringPtr("test")
			}
			rg := getResourceGroup(tc.rgName, tags)
			age, ok := shouldDeleteResourceGroup(rg, defaultTTL)
			if ok != tc.expectedToBeDeleted {
				t.Fatalf("expected %t, but got %t", tc.expectedToBeDeleted, ok)
			}
			if age != tc.expectedAge {
				t.Fatalf("expected the resource group age to be '%s', but got '%s'", tc.expectedAge, age)
			}
		})
	}
}

func getResourceGroup(name string, tags map[string]*string) resources.Group {
	return resources.Group{
		Name: to.StringPtr(name),
		Tags: tags,
	}
}

package main

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/go-autorest/autorest/to"
)

func TestShouldDeleteResourceGroup(t *testing.T) {
	oneDayAgo := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	fourDaysAgo := time.Now().Add(-defaultTTL - 24*time.Hour).Format(time.RFC3339)
	oneDayAgeOutput := "1 days (24 hours)"
	fourDayAgeOutput := "4 days (96 hours)"
	testCases := []struct {
		desc                string
		rgName              string
		creationTimestamp   string
		hasDoNotDelete      bool
		expectedToBeDeleted bool
		expectedAge         string
		regex               string
	}{
		{
			desc:                "deletable resource group that has not lived for more than 3 days",
			rgName:              "kubetest-123",
			creationTimestamp:   oneDayAgo,
			expectedToBeDeleted: false,
			expectedAge:         oneDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "kubetest resource group that lives for more than 3 days",
			rgName:              "kubetest-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "azuredisk-csi-driver resource group that lives for more than 3 days",
			rgName:              "azuredisk-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "azurefile-csi-driver resource group that lives for more than 3 days",
			rgName:              "azurefile-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "blobfuse-csi-driver resource group that lives for more than 3 days",
			rgName:              "blobfuse-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "blobfuse-csi-driver resource group that lives for more than 3 days",
			rgName:              "blobfuse-csi-driver-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "flannel resource group that lives for more than 3 days",
			rgName:              "flannel-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "ctrd resource group that lives for more than 3 days",
			rgName:              "ctrd-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "capz resource group that lives for more than 3 days",
			rgName:              "capz-456",
			creationTimestamp:   fourDaysAgo,
			expectedToBeDeleted: true,
			expectedAge:         fourDayAgeOutput,
			regex:               "",
		},
		{
			desc:                "deletable resource group with no creation timestamp",
			rgName:              "kubetest-789",
			creationTimestamp:   "",
			expectedToBeDeleted: true,
			expectedAge:         "probably a long time because it does not have a 'creationTimestamp' tag. Found tags: map[]",
			regex:               "",
		},
		{
			desc:                "deletable resource group with invalid creation timestamp",
			rgName:              "kubetest-789",
			creationTimestamp:   "invalid creation timestamp",
			expectedToBeDeleted: false,
			expectedAge:         "",
			regex:               "",
		},
		{
			desc:                "deletable resource group but has a DO-NOT-DELETE tag",
			rgName:              "kubetest-789",
			creationTimestamp:   fourDaysAgo,
			hasDoNotDelete:      true,
			expectedToBeDeleted: false,
			expectedAge:         "",
			regex:               "",
		},
		{
			desc:                "deletable resource group and matches regex",
			rgName:              "kubetest-fake-123",
			creationTimestamp:   "",
			hasDoNotDelete:      false,
			expectedToBeDeleted: true,
			expectedAge:         "probably a long time because it does not have a 'creationTimestamp' tag. Found tags: map[]",
			regex:               "^kubetest.+$",
		},
		{
			desc:                "resource group no deletable doesn't match full name to regex",
			rgName:              "kubetest-fake-123",
			creationTimestamp:   fourDaysAgo,
			hasDoNotDelete:      false,
			expectedToBeDeleted: false,
			expectedAge:         "",
			regex:               "kubetest",
		},
		{
			desc:                "resource group no deletable, matches regex but has DO-NOT-DELETE",
			rgName:              "kubetest-other",
			creationTimestamp:   "",
			hasDoNotDelete:      true,
			expectedToBeDeleted: false,
			expectedAge:         "",
			regex:               "^.+$",
		},
		{
			desc:                "deletable resource group that matches regex and has not lived for more than 3 days",
			rgName:              "kubetest-new",
			creationTimestamp:   oneDayAgo,
			expectedToBeDeleted: false,
			expectedAge:         oneDayAgeOutput,
			regex:               "^kube.+",
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
			age, ok := shouldDeleteResourceGroup(&rg, defaultTTL, tc.regex)
			if ok != tc.expectedToBeDeleted {
				t.Fatalf("expected %t, but got %t", tc.expectedToBeDeleted, ok)
			}
			if age != tc.expectedAge {
				t.Fatalf("expected the resource group age to be '%s', but got '%s'", tc.expectedAge, age)
			}
		})
	}
}

func getResourceGroup(name string, tags map[string]*string) armresources.ResourceGroup {
	return armresources.ResourceGroup{
		Name: to.StringPtr(name),
		Tags: tags,
	}
}

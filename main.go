package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/oracle/oci-go-sdk/example/helpers"
	"github.com/oracle/oci-go-sdk/identity"
)

type VM struct {
	DisplayName, OCID, CompartmentID, Region string
}

//check error helper function
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

//ConfigGen generates a new config from the defult config with a new region

func ConfigGen(c common.ConfigurationProvider, region string) common.ConfigurationProvider {

	keylocation := os.Getenv("HOME") + "/.oci/oci_api_key.pem"
	key, err := ioutil.ReadFile(keylocation)
	check(err)

	tenancyID, err := c.TenancyOCID()
	check(err)
	userID, err := c.UserOCID()
	check(err)
	fingerprint, err := c.KeyFingerprint()
	check(err)

	return common.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, string(key), common.String(""))
}

func GetSuscribedRegions(c common.ConfigurationProvider) []string {

	var susbcribedRegions []string

	tenancyID, err := c.TenancyOCID()
	if err != nil {
		log.Fatal(err)
	}
	req := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(tenancyID)}

	client, err := identity.NewIdentityClientWithConfigurationProvider(c)
	if err != nil {
		panic(err)
	}

	response, err := client.ListRegionSubscriptions(context.Background(), req)
	if err != nil {
		log.Fatal(err)
	}
	for _, v := range response.Items {
		susbcribedRegions = append(susbcribedRegions, *v.RegionName)
	}

	return susbcribedRegions
}

//GetCompartments Scans all compartments in tenancy
func GetAllCompartments(c common.ConfigurationProvider) []string {

	var compartmentIDs []string
	// The OCID of the tenancy containing the compartment.
	tenancyID, err := c.TenancyOCID()
	if err != nil {
		log.Fatal(err)
	}

	//traverse all compartments and its sub-compartments
	subtree := true
	req := identity.ListCompartmentsRequest{
		CompartmentId:          &tenancyID,
		AccessLevel:            "ANY",
		CompartmentIdInSubtree: &subtree,
		LifecycleState:         "ACTIVE",
	}

	client, err := identity.NewIdentityClientWithConfigurationProvider(c)
	if err != nil {
		panic(err)
	}
	//List Compartments
	response, _ := client.ListCompartments(context.Background(), req)

	for _, v := range response.Items {
		compartmentIDs = append(compartmentIDs, *v.Id)
	}
	return compartmentIDs
}

//ScanVms
func ScanVms(c common.ConfigurationProvider, compartments, regions []string) map[string]VM {

	servers := make(map[string]VM)
	//regions := GetSuscribedRegions(c)
	for _, v := range regions {
		config := ConfigGen(c, v)
		client, err := core.NewComputeClientWithConfigurationProvider(config)
		helpers.FatalIfError(err) // return error

		//
		listComputeFunc := func(request core.ListInstancesRequest) (core.ListInstancesResponse, error) {
			return client.ListInstances(context.Background(), request)
		}

		//Prevent Retry Policy Error
		requestMetadata := common.RequestMetadata{
			RetryPolicy: &common.RetryPolicy{
				MaximumNumberAttempts: 10,
				ShouldRetryOperation: func(res common.OCIOperationResponse) bool {
					if res.Error != nil {
						return true
					}
					return false
				},
				NextDuration: func(common.OCIOperationResponse) time.Duration {
					return 2 * time.Second
				},
			},
		}
		for _, cid := range compartments {
			req := core.ListInstancesRequest{CompartmentId: common.String(cid), RequestMetadata: requestMetadata}
			for resp, err := listComputeFunc(req); ; resp, err = listComputeFunc(req) {
				helpers.FatalIfError(err)

				for _, vm := range resp.Items {
					if vm.LifecycleState != core.InstanceLifecycleStateTerminated && vm.LifecycleState != core.InstanceLifecycleStateTerminating {
						servers[*vm.DisplayName] = VM{*vm.DisplayName, *vm.Id, *vm.CompartmentId, v}
					}
				}

				if resp.OpcNextPage != nil {
					// if there are more items in next page, fetch items from next page
					req.Page = resp.OpcNextPage
				} else {
					// no more result, break the loop
					break
				}
			}

		}

	}
	return servers
}

func main() {

	dconfig := common.DefaultConfigProvider()
	compartments := GetAllCompartments(dconfig)
	regions := GetSuscribedRegions(dconfig)

	servers := ScanVms(dconfig, compartments, regions)
	fmt.Println(servers["vmOps"])

}

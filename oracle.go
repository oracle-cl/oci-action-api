package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/identity"
)

const (
	config_path = "oci\\config"
)

func ExampleListAvailabilityDomains() {

	config, _ := common.ConfigurationProviderFromFile(config_path, "")
	c, err := identity.NewIdentityClientWithConfigurationProvider(config)
	fmt.Println(err)

	// The OCID of the tenancy containing the compartment.
	tenancyID, err := common.DefaultConfigProvider().TenancyOCID()
	fmt.Println(err)

	request := identity.ListAvailabilityDomainsRequest{
		CompartmentId: &tenancyID,
	}

	r, err := c.ListAvailabilityDomains(context.Background(), request)
	fmt.Println(err)

	log.Printf("list of available domains: %v", r.Items)
	fmt.Println("list available domains completed")

	// Output:
	// list available domains completed
}

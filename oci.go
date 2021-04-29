package main

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/oracle/oci-go-sdk/identity"
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getPrivKeylocation(configpath string) string {
	f, err := os.Open(configpath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Splits on newlines by default.
	scanner := bufio.NewScanner(f)

	// https://golang.org/pkg/bufio/#Scanner.Scan
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "key_file") {
			return strings.Split(scanner.Text(), "=")[1]
		}

	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return ""
}

//Find all profiles available in the default config file
func findProfiles(configLocation string) []string {

	var profiles []string

	config, err := ioutil.ReadFile(configLocation)
	if err != nil {
		log.Fatal(err)
	}

	text := string(config)

	re := regexp.MustCompile(`\[(.*?)\]`)

	matches := re.FindAllStringSubmatch(text, -1)
	for _, p := range matches {
		profiles = append(profiles, p[1])
	}

	return profiles
}

type connection struct {
	config common.ConfigurationProvider
}

//ConfigGen generates a new config from the defult config with a new region
func (c *connection) ConfigGen(region string) common.ConfigurationProvider {

	//Find key and read it
	pwd, err := os.Getwd()
	check(err)
	keylocation := getPrivKeylocation(pwd + "/config")
	if keylocation == "" {
		log.Fatal("No Keyfile location found in config")
	}
	key, err := ioutil.ReadFile(keylocation)
	check(err)

	//Config Details
	tenancyID, err := c.config.TenancyOCID()
	check(err)
	userID, err := c.config.UserOCID()
	check(err)
	fingerprint, err := c.config.KeyFingerprint()
	check(err)

	return common.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, string(key), common.String(""))
}

func (c *connection) GetSuscribedRegions() ([]string, error) {

	var susbcribedRegions []string

	tenancyID, err := c.config.TenancyOCID()
	if err != nil {
		return []string{}, err
	}
	req := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(tenancyID)}

	client, err := identity.NewIdentityClientWithConfigurationProvider(c.config)
	if err != nil {
		return []string{}, err
	}

	response, err := client.ListRegionSubscriptions(context.Background(), req)
	if err != nil {
		return []string{}, err
	}

	for _, v := range response.Items {
		susbcribedRegions = append(susbcribedRegions, *v.RegionName)
	}

	return susbcribedRegions, nil
}

//GetCompartments Scans all compartments in tenancy
func (c *connection) GetAllCompartments() []string {

	var compartmentIDs []string
	// The OCID of the tenancy containing the compartment.
	tenancyID, err := c.config.TenancyOCID()
	if err != nil {
		log.Fatal(err)
	}

	//traverse all compartments and its sub-compartments
	subtree := true
	req := identity.ListCompartmentsRequest{
		CompartmentId:          common.String(tenancyID),
		AccessLevel:            "ANY",
		CompartmentIdInSubtree: &subtree,
		LifecycleState:         "ACTIVE",
	}
	client, err := identity.NewIdentityClientWithConfigurationProvider(c.config)
	if err != nil {
		log.Fatal(err)
	}

	//List Compartments
	response, _ := client.ListCompartments(context.Background(), req)

	for _, v := range response.Items {
		compartmentIDs = append(compartmentIDs, *v.Id)
	}
	return compartmentIDs
}

type VM struct {
	DisplayName   string `json:"name"`
	OCID          string `json:"ocid"`
	CompartmentID string `json:"compartment_id"`
	Region        string `json:"region"`
	Status        string `json:"status"`
}

//ScanVms will go throug all regions and compartments to get Active Compute instances
func (c *connection) ScanVms(compartments, regions []string) map[string]VM {

	servers := make(map[string]VM)
	//regions := GetSuscribedRegions(c)
	for _, r := range regions {
		config := c.ConfigGen(r)
		client, err := core.NewComputeClientWithConfigurationProvider(config)
		if err != nil {
			log.Fatal(err)
		}

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
				if err != nil {
					log.Fatal(err)
				}

				for _, vm := range resp.Items {
					if vm.LifecycleState != core.InstanceLifecycleStateTerminated && vm.LifecycleState != core.InstanceLifecycleStateTerminating {
						servers[*vm.DisplayName] = VM{*vm.DisplayName, *vm.Id, *vm.CompartmentId, r, string(vm.LifecycleState)}
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

//Action given by vm and action
func (c *connection) Action(action string, vm VM) error {

	//Check if action is recognized
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unrecognize action: %s,", action)
	}

	switch action {
	case "stop":
		action = strings.ToUpper("softstop")
	case "restart":
		action = strings.ToUpper("softrestart")
	case "start":
		action = strings.ToUpper(action)
	}

	newconfig := c.ConfigGen(vm.Region)
	client, err := core.NewComputeClientWithConfigurationProvider(newconfig)
	if err != nil {
		return err
	}

	req := core.InstanceActionRequest{
		InstanceId: common.String(vm.OCID),
		Action:     core.InstanceActionActionEnum(action),
	}
	_, err = client.InstanceAction(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

func scanAllProfiles() map[string]map[string]VM {

	current, _ := os.Getwd()
	cfgfile := current + "/config"
	profiles := findProfiles(cfgfile)
	fmt.Println(profiles)

	profileSrvs := make(map[string]map[string]VM)

	//Star scaning from every profile in the config file
	for _, p := range profiles {
		cfg, err := common.ConfigurationProviderFromFileWithProfile(cfgfile, p, "")
		check(err)

		//Create connection
		conn := connection{cfg}
		log.Printf("Scaning Tenant in profile: %v", p)

		//Get all compartments in tenancy
		compartments := conn.GetAllCompartments()
		log.Printf("%v comparments found", len(compartments))

		//Get all suscribed regions
		regions, err := conn.GetSuscribedRegions()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Subscribed Regions: %v", regions)
		servers := conn.ScanVms(compartments, regions)
		profileSrvs[p] = servers

	}

	return profileSrvs

}
func main() {

	all := scanAllProfiles()
	fmt.Println(all)

}

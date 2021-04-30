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

	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/identity"
)

//Helper Functions
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func Find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

//OCI config file struct
type config struct {
	location string
	profile  string
}

func (cfg config) profileExist() bool {

	if cfg.profile == "" {
		return false
	}

	_, found := Find(cfg.getAllProfiles(), cfg.profile)
	if found {
		return true
	}

	return false
}

func (cfg *config) defaultProfile() {
	if cfg.profile == "" {
		cfg.profile = "DEFAULT"
	}
}

func (cfg *config) getPrivKeylocation() string {

	//Check if default profile
	cfg.defaultProfile()

	f, err := os.Open(cfg.location)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Splits on newlines by default.
	scanner := bufio.NewScanner(f)

	p := 0
	// https://golang.org/pkg/bufio/#Scanner.Scan
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), cfg.profile) {
			fmt.Println("Profile found")
			p++
		}
		if strings.Contains(scanner.Text(), "key_file") && p > 0 {
			return strings.Split(scanner.Text(), "=")[1]
		}

	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return ""
}

//Find all profiles available in the config file
func (cfg *config) getAllProfiles() []string {

	var profiles []string

	config, err := ioutil.ReadFile(cfg.location)
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

func (cfg *config) gen() common.ConfigurationProvider {

	//set default profile in case profile is empty
	cfg.defaultProfile()

	c, err := common.ConfigurationProviderFromFileWithProfile(cfg.location, cfg.profile, "")
	//Error will raise if default profile cannot be loaded
	check(err)
	return c
}

func (cfg *config) genByRegion(region string) common.ConfigurationProvider {

	//load config
	cp := cfg.gen()

	//Find key and read it
	keylocation := cfg.getPrivKeylocation()
	if keylocation == "" {
		log.Fatal("No Keyfile location found in config")
	}
	key, err := ioutil.ReadFile(keylocation)
	check(err)

	//Config Details
	tenancyID, err := cp.TenancyOCID()
	check(err)
	userID, err := cp.UserOCID()
	check(err)
	fingerprint, err := cp.KeyFingerprint()
	check(err)

	return common.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, string(key), common.String(""))

}

type connect struct {
	conn common.ConfigurationProvider
	config
}

func (oci *connect) GetSuscribedRegions() ([]string, error) {

	var susbcribedRegions []string

	tenancyID, err := oci.conn.TenancyOCID()
	if err != nil {
		return []string{}, err
	}
	req := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(tenancyID)}

	client, err := identity.NewIdentityClientWithConfigurationProvider(oci.conn)
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
func (oci *connect) GetAllCompartments() []string {

	var compartmentIDs []string
	// The OCID of the tenancy containing the compartment.
	tenancyID, err := oci.conn.TenancyOCID()
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
	client, err := identity.NewIdentityClientWithConfigurationProvider(oci.conn)
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
	Profile       string `json:"profile"`
}

//ScanVms will go throug all regions and compartments to get Active Compute instances
/* func (oci *connect) ScanVms() map[string]VM {

	servers := make(map[string]VM)
	//regions := GetSuscribedRegions(c)
	compartments := oci.GetAllCompartments()
	regions, err := oci.GetSuscribedRegions()
	check(err)

	for _, r := range regions {
		//config := c.ConfigGen(r)
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
*/

func main() {

	cfg := config{
		location: "/home/dave/code/oci-action-api/config",
	}

	fmt.Println(cfg.getPrivKeylocation())
}

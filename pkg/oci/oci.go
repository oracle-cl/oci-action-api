package oci

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
type Config struct {
	Location string
	Profile  string
}

func (cfg Config) ProfileExist() bool {

	if cfg.Profile == "" {
		return false
	}

	_, found := Find(cfg.GetAllProfiles(), cfg.Profile)
	if !found {
		return false
	} else {
		return true
	}

}

func (cfg *Config) DefaultProfile() {
	if cfg.Profile == "" {
		cfg.Profile = "DEFAULT"
	}
}

func (cfg *Config) GetPrivKeylocation() string {

	//Check if default profile
	cfg.DefaultProfile()

	f, err := os.Open(cfg.Location)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Splits on newlines by default.
	scanner := bufio.NewScanner(f)

	p := 0
	// https://golang.org/pkg/bufio/#Scanner.Scan
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), cfg.Profile) {
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
func (cfg *Config) GetAllProfiles() []string {

	var profiles []string

	config, err := ioutil.ReadFile(cfg.Location)
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

func (cfg *Config) Gen() common.ConfigurationProvider {

	//set default profile in case profile is empty
	cfg.DefaultProfile()

	c, err := common.ConfigurationProviderFromFileWithProfile(cfg.Location, cfg.Profile, "")
	//Error will raise if default profile cannot be loaded
	check(err)
	return c
}

func (cfg *Config) GenByRegion(region string) common.ConfigurationProvider {

	//load config
	cp := cfg.Gen()

	//Find key and read it
	keylocation := cfg.GetPrivKeylocation()
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

func GetSuscribedRegions(conn common.ConfigurationProvider) ([]string, error) {

	var susbcribedRegions []string

	tenancyID, err := conn.TenancyOCID()
	if err != nil {
		return []string{}, err
	}
	req := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(tenancyID)}

	client, err := identity.NewIdentityClientWithConfigurationProvider(conn)
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

	log.Printf("Suscribed Regions: %v", susbcribedRegions)

	return susbcribedRegions, nil
}

//GetCompartments Scans all compartments in tenancy
func GetAllCompartments(conn common.ConfigurationProvider) []COMPA {

	var compartmentIDs []COMPA
	// The OCID of the tenancy containing the compartment.
	tenancyID, err := conn.TenancyOCID()
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
	client, err := identity.NewIdentityClientWithConfigurationProvider(conn)
	if err != nil {
		log.Fatal(err)
	}

	//List Compartments
        for {
           response, _ := client.ListCompartments(context.Background(), req)
	   for _, v := range response.Items {
                log.Printf("compartment %v %v found parent %v", *v.Name, *v.Id, *v.CompartmentId)
	        compartmentIDs = append(compartmentIDs, COMPA{ *v.Id, *v.Name, *v.CompartmentId,tenancyID})
	   }
           if response.OpcNextPage != nil { req.Page = response.OpcNextPage } else { break }
        }

	log.Printf("%v compartments found", len(compartmentIDs))
	return compartmentIDs
}

func GetPath( c []COMPA, ocid string) string {
        if c[0].TenancyID == ocid {
                return "root"
        }
	for _, item := range c {
		if item.CompartmentID == ocid {
			return GetPath(c,item.CompartmentParent) + "/" + item.CompartmentName
		}
	}
	log.Printf("ERROR: compartment not found %v", ocid)
        return "error"
}

//ScanVms will go throug all regions and compartments to get Active Compute instances
func (cfg *Config) ScanVms() []VM {

	conn := cfg.Gen()

	var servers []VM
	//regions := GetSuscribedRegions(c)
	compartments := GetAllCompartments(conn)
	regions, err := GetSuscribedRegions(conn)
	check(err)

	log.Printf("Start scanning for virtual machines in profile: %v", cfg.Profile)
	for _, r := range regions {
		config := cfg.GenByRegion(r)
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
		  req := core.ListInstancesRequest{CompartmentId: common.String(cid.CompartmentID), RequestMetadata: requestMetadata}
		  for resp, err := listComputeFunc(req); ; resp, err = listComputeFunc(req) {
		  if err != nil {
			log.Fatal(err)
		  }
		  for _, vm := range resp.Items {
			if vm.LifecycleState != core.InstanceLifecycleStateTerminated && vm.LifecycleState != core.InstanceLifecycleStateTerminating {
			comp_path := GetPath( compartments, *vm.CompartmentId)
			servers = append(servers, VM{strings.ToLower(*vm.DisplayName), *vm.Id, *vm.CompartmentId, cid.CompartmentName, comp_path, *vm.Region, string(vm.LifecycleState), cfg.Profile})
                        log.Printf("machine added: %v comp:%v full:%v", strings.ToLower(*vm.DisplayName), cid.CompartmentName, comp_path)
			}
		  }
		  if resp.OpcNextPage != nil { req.Page = resp.OpcNextPage } else { break }
		  }
		}
	}
	log.Printf("Number of Virtual Machines Found: %v", len(servers))
	return servers
}

type VM struct {
	DisplayName     string `json:"name"`
	OCID            string `json:"ocid"`
	CompartmentID   string `json:"compartment_id"`
	CompartmentName string `json:"compartment_name"`
	CompartmentPath string `json:"compartment_path"`
	Region          string `json:"region"`
	Status          string `json:"status"`
	Profile         string `json:"profile"`
}
type COMPA struct {
	CompartmentID     string `json:"compartment_id"`
	CompartmentName   string `json:"compartment_name"`
	CompartmentParent string `json:"compartment_parent"`
	TenancyID         string `json:"tenancy_id"`
}

//Get vm
func (cfg *Config) GetVM(vm VM) (VM, error) {

	//set profile by vm
	cfg.Profile = vm.Profile
	conn := cfg.GenByRegion(vm.Region)

	client, err := core.NewComputeClientWithConfigurationProvider(conn)
	if err != nil {
		return VM{}, err
	}

        req := core.GetInstanceRequest{
		InstanceId:      &vm.OCID,
	}

	resp, err := client.GetInstance(context.Background(), req)
	if err != nil {
		return VM{}, err
	}

	//VM
	server := VM{
		DisplayName:   *resp.DisplayName,
		CompartmentID: *resp.CompartmentId,
		CompartmentName: vm.CompartmentName,
		CompartmentPath: vm.CompartmentPath,
		OCID:          *resp.Id,
		Region:        vm.Region,
		Status:        string(resp.LifecycleState),
		Profile:       vm.Profile,
	}
	return server, nil
}

//Action given by vm and action
func (cfg *Config) Action(action string, vm VM) error {

	log.Printf("Starting action %v on %v", action, vm.DisplayName)
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

	conn := cfg.GenByRegion(vm.Region)
	client, err := core.NewComputeClientWithConfigurationProvider(conn)
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

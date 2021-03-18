package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/oracle/oci-go-sdk/identity"
)

//check error helper function
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type Connection struct {
	Config common.ConfigurationProvider
}

//ConfigGen generates a new config from the defult config with a new region
func (c *Connection) ConfigGen(region string) common.ConfigurationProvider {

	pwd, err := os.Getwd()
	check(err)
	keylocation := pwd + "/oci_api_key.pem"
	key, err := ioutil.ReadFile(keylocation)
	check(err)

	//Config Details
	tenancyID, err := c.Config.TenancyOCID()
	check(err)
	userID, err := c.Config.UserOCID()
	check(err)
	fingerprint, err := c.Config.KeyFingerprint()
	check(err)

	return common.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, string(key), common.String(""))
}

func (c *Connection) GetSuscribedRegions() ([]string, error) {

	var susbcribedRegions []string

	tenancyID, err := c.Config.TenancyOCID()
	if err != nil {
		return []string{}, err
	}
	req := identity.ListRegionSubscriptionsRequest{TenancyId: common.String(tenancyID)}

	client, err := identity.NewIdentityClientWithConfigurationProvider(c.Config)
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
func (c *Connection) GetAllCompartments() []string {

	var compartmentIDs []string
	// The OCID of the tenancy containing the compartment.
	tenancyID, err := c.Config.TenancyOCID()
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
	client, err := identity.NewIdentityClientWithConfigurationProvider(c.Config)
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
}

//ScanVms will go throug all regions and compartments to get Active Compute instances
func (c *Connection) ScanVms(compartments, regions []string) map[string]VM {

	servers := make(map[string]VM)
	//regions := GetSuscribedRegions(c)
	for _, v := range regions {
		config := c.ConfigGen(v)
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

//Action given by vm and action
func (c *Connection) Action(action string, vm VM) error {

	//Check if action is recognized
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unrecognize action: %s,", action)
	}

	switch action {
	case "stop":
		action = "sofstop"
	case "restart":
		action = "softrestart"
	}

	if region, _ := c.Config.Region(); region != vm.Region {

		newconfig := c.ConfigGen(vm.Region)
		client, err := core.NewComputeClientWithConfigurationProvider(newconfig)
		if err != nil {
			return err
		}

		req := core.InstanceActionRequest{
			InstanceId: common.String(vm.OCID),
			Action:     core.InstanceActionActionEnum(strings.ToUpper(action)),
		}
		_, err = client.InstanceAction(context.Background(), req)
		if err != nil {
			return err
		}

	}
	return nil
}

/*
type VMHandlers struct {
	sync.Mutex
	store map[string]VM
}

func (h *VMHandlers) oci(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.Get(w, r)
		return
	case "POST":
		h.Post(w, r)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("method not allowed"))
		return
	}
}

func (h *VMHandlers) Get(w http.ResponseWriter, r *http.Request) {
}

func (h *VMHandlers) Post(w http.ResponseWriter, r *http.Request) {

	h.Lock()
	defer h.Unlock()

}
*/
/* func newVmHandlers() *VMHandlers {
	return &VMHandlers{
		store: map[string]VM{},
	}
}
*/
func main() {

	current, _ := os.Getwd()
	config, err := common.ConfigurationProviderFromFile(current+"/config", "")

	check(err)
	conn := Connection{config}
	compartments := conn.GetAllCompartments()
	check(err)
	regions, err := conn.GetSuscribedRegions()
	check(err)

	servers := conn.ScanVms(compartments, regions)
	check(err)
	fmt.Println(servers["vmOps"])

	/* VMHandlers := newVmHandlers()

	http.HandleFunc("/oci", VMHandlers.Get)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	} */

}

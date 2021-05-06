package main

import (
	"log"
	"os"

	"github.com/davejfranco/oci-action-api/pkg/oci"
)

const (
	configLocation = "config"
)

func scanAll() {

	rhost := os.Getenv("RHOST")
	rport := os.Getenv("RPORT")

	//Default redis host and port
	if rhost == "" {
		rhost = "localhost"
	}

	if rport == "" {
		rport = "6379"
	}

	log.Println("Start scanning")
	config := oci.Config{Location: configLocation}

	//Get all profiles available in the config file
	profiles := config.GetAllProfiles()
	log.Printf("Profiles found in config file: %v", profiles)

	for _, profile := range profiles {
		config.Profile = profile

		db := oci.Store{
			Address: rhost,
			Port:    rport,
		}

		//Connect to Database
		err := db.Connect()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		servers := config.ScanVms()

		//Flush all keys
		db.FlushAll()

		//Insert all vms
		db.Set(&servers)
	}
}

func main() {

	//Scan all VMs in all tenants, regions and compartments
	scanAll()
}

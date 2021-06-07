package main

import (
	"log"
	"os"
	"time"

	"github.com/davejfranco/oci-action-api/pkg/oci"
)

const (
	configLocation = "config"
)

func scanAll() {

	log.Println("################ Worker Start #############################")
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

	//Database object
	db := oci.Store{
		Address: rhost,
		Port:    rport,
	}

	//Connect to Database
	err := db.Connect()
	if err != nil {
		log.Fatal(err)
	}

	//Flush database - I should manage this in a more fancy way
	log.Println("flushing database cache.")
	err = db.FlushAll()
	if err != nil {
		log.Fatal(err)
	}

	//Close connection at the end
	defer db.Close()

	for _, profile := range profiles {
		config.Profile = profile

		servers := config.ScanVms()

		//Insert all vms
		err = db.Set(&servers)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("################ Worker End #############################")
}

func init() {

	//init Scan everything
	scanAll()
}

func main() {

	ticker := time.NewTicker(24 * time.Hour)

	//Scan all VMs in all tenants, regions and compartments
	for _ = range ticker.C {
		scanAll()
	}

}

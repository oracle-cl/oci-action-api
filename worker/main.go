package main

import (
	"log"

	"github.com/davejfranco/oci-action-api/pkg/oci"
)

const (
	configLocation = "config"
)

func scanAll() {

	log.Println("Start scanning")
	config := oci.Config{Location: configLocation}

	//Get all profiles available in the config file
	profiles := config.GetAllProfiles()
	log.Printf("Profiles found in config file: %v", profiles)

	for _, profile := range profiles {
		config.Profile = profile

		db := oci.Store{
			Address: "localhost",
			Port:    "6379",
		}

		//Connect to Database
		err := db.Connect()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		servers := config.ScanVms()
		db.Set(&servers)
	}
}

func main() {

	scanAll()
}

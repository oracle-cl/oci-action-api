package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"sync"

	"github.com/davejfranco/oci-action-api/pkg/oci"
	"github.com/oracle/oci-go-sdk/common"
)

type VMHandlers struct {
	sync.Mutex
	db     oci.Store
	config oci.Config
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
	query := r.URL.Query()

	name := query.Get("name")
	if name == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	//Connect to redis
	err := h.db.Connect()
	if err != nil {
		log.Fatal(err)
	}

	//close connection
	defer h.db.Close()

	//find vm in database
	vm := h.db.Get(name)
	if vm == (oci.VM{}) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	server, err := h.config.GetVM(vm)
	if err != nil {
		log.Fatal(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err = h.db.Update(&server)
	if err != nil {
		log.Fatal(err)
	}

	jsonBytes, _ := json.Marshal(server)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)

}

func (h *VMHandlers) Post(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	name := query.Get("name")
	action := query.Get("action")
	//check if name or action exists
	if action == "" && name == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ct := r.Header.Get("content-type")
	if ct != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		w.Write([]byte(fmt.Sprintf("need content-type 'application/json', but got '%s'", ct)))
		return
	}

	h.Lock()
	defer h.Unlock()
	server, ok := h.store[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := h.conn.Action(action, server)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		log.Fatal(err)
		return
	}
	log.Printf("Action: %v initiate on Server: %v", action, name)
	jsonBytes, _ := json.Marshal(server)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)

}

func newVmHandlers() *VMHandlers {
	current, _ := os.Getwd()
	config, err := common.ConfigurationProviderFromFile(current+"/config", "")
	if err != nil {
		log.Fatal(err)
	}

	//initialize connection to OCI
	newconn := connection{config}
	log.Print("Scanning OCI Tenant with provided config")
	//Remember to implement periodic Sync
	compartments := newconn.GetAllCompartments()
	log.Printf("%v comparments found", len(compartments))
	if err != nil {
		log.Fatal(err)
	}
	regions, err := newconn.GetSuscribedRegions()
	log.Printf("Subscribed Regions: %v", regions)
	if err != nil {
		log.Fatal(err)
	}

	vms := newconn.ScanVms(compartments, regions)
	log.Printf("%v virtual machines found", len(vms))
	log.Print("Done Scanning")
	return &VMHandlers{
		store: vms,
		conn:  newconn,
	}
}

/* func main() {

	VMHandlers := newVmHandlers()
	http.HandleFunc("/oci", VMHandlers.oci)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}

} */

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"sync"

	"github.com/davejfranco/oci-action-api/pkg/oci"
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
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "method not allowed")))
		return
	}
}

func (h *VMHandlers) Get(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	name := strings.ToLower(query.Get("name"))
	log.Println(fmt.Sprintf("Get : %v", name))
	if name == "" {
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
		return
	}

	h.Lock()
	defer h.Unlock()
	//Connect to redis
	err := h.db.Connect()
	if err != nil {
	        log.Println("Error en db.Connect")
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
                return
	}

	//close connection
	defer h.db.Close()

	//find vm in database
	vm := h.db.Get(name)
	if vm == (oci.VM{}) {
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
		return
	}

	//Get server to compare with db
	server, err := h.config.GetVM(vm)
	if err != nil {
	        log.Println("Error en db.Connect")
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
		return
	}

	//If Servers is not found in OCI account delete it from DB
	if server == (oci.VM{}) {
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
		_ = h.db.Delete(name)
		return
	}

	err = h.db.Update(&server)
	if err != nil {
	        log.Println("Error en db.Update")
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
                return
	}

	jsonBytes, _ := json.Marshal(server)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)

}

type actionReq struct {
	Name          string
	Compartment   string
	Action        string
}

//Check if action is a valid one
func (a *actionReq) isvalid() bool {

	switch a.Action {
	case "start":
		return true
	case "stop":
		return true
	default:
		return false
	}

}
func (h *VMHandlers) Post(w http.ResponseWriter, r *http.Request) {

	//{"name":"MyVM", "compartment": "root/comp1/comp2", "action":"start"}'
	var req actionReq

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Println(fmt.Sprintf("Post : %v", req.Name))
	if req.Name == "" || req.Action == "" {
		w.WriteHeader(http.StatusBadRequest)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "bad request null")))
		return
	}

	if !req.isvalid() {
		w.WriteHeader(http.StatusBadRequest)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "bad request")))
		return
	}

	//ct := r.Header.Get("content-type")
	//if ct != "application/json" {
		//w.WriteHeader(http.StatusUnsupportedMediaType)
		//w.Write([]byte(fmt.Sprintf("need content-type 'application/json', but got '%s'", ct)))
		//return
	//}

	h.Lock()
	defer h.Unlock()

	//Connect to redis
	err = h.db.Connect()
	if err != nil {
	        log.Println("Error en db.connect redis")
		w.WriteHeader(http.StatusNotFound)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found")))
                return
	}

	//close connection
	defer h.db.Close()

	//find vm in database
	srv := h.db.Get(strings.ToLower(req.Name))
	if srv == (oci.VM{}) {
		w.WriteHeader(http.StatusNotFound)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "not found in db")))
		return
	}

        //validate compartment
	if strings.ToLower(srv.CompartmentPath) != strings.ToLower(req.Compartment) {
	        log.Println("Invalid Compartment")
	        w.Header().Add("content-type", "application/json")
	        w.WriteHeader(http.StatusOK)
                w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"Invalid Compartment"}`)))
		return
	}

	//set profile
	h.config.Profile = srv.Profile

	err = h.config.Action(req.Action, srv)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"status":"false","msg":"%v"}`, "Acction Error")))
		return
	}

	log.Printf("Action: %v initiate on Server: %v", req.Action, req.Name)
	//jsonBytes, _ := json.Marshal(srv)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"status":"true","msg":"%v"}`, "ok")))

}

func newVmHandlers() *VMHandlers {

	cfg := oci.Config{
		Location: "config",
	}

	//redis host and ports
	rhost := os.Getenv("RHOST")
	rport := os.Getenv("RPORT")

	//Default redis host and port
	if rhost == "" {
		rhost = "localhost"
	}

	if rport == "" {
		rport = "6379"
	}

	str := oci.Store{
		Address: rhost,
		Port:    rport,
	}

	return &VMHandlers{
		db:     str,
		config: cfg,
	}
}

func main() {

	log.Println("Server Started...")
	VMHandlers := newVmHandlers()
	http.HandleFunc("/oci", VMHandlers.oci)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}

}

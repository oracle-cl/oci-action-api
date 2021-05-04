package oci

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/nitishm/go-rejson/v4"
)

type Store struct {
	Address  string
	Port     string
	conn     redis.Conn
	rHandler rejson.Handler
}

func (s *Store) redisString() string {
	return strings.Join([]string{s.Address, s.Port}, ":")
}

func (s *Store) Connect() error {

	addr := s.redisString()
	temp, err := redis.Dial("tcp", addr)
	if err != nil {
		return err
	}

	s.conn = temp

	//Set Redis Handler
	s.rHandler = *rejson.NewReJSONHandler()
	s.rHandler.SetRedigoClient(s.conn)

	return nil
}

/* func (s *Store) redisHandler() *rejson.Handler {

	rh := rejson.NewReJSONHandler()
	rh.SetRedigoClient(s.conn)
	return rh
} */
func (s *Store) Close() {

	err := s.conn.Close()
	if err != nil {
		log.Fatalf("Failed to communicate to redis-server @ %v", err)
	}
}

func (s *Store) Set(vms *[]VM) {

	//rh := rejson.NewReJSONHandler()
	//rh.SetRedigoClient(s.conn)
	//rh := s.redisHandler()

	for _, vm := range *vms {

		res, err := s.rHandler.JSONSet(vm.DisplayName, ".", vm)
		if err != nil {
			log.Fatal("Failed to JSONSet")
		}

		if res.(string) != "OK" {
			if err != nil {
				log.Fatalf("Failed to Set %v", vm)
			}
		}
	}

}

func (s *Store) Get(vm_name string) VM {
	//rh := rejson.NewReJSONHandler()
	//rh.SetRedigoClient(s.conn)
	//rh := s.redisHandler()

	vmJSON, err := redis.Bytes(s.rHandler.JSONGet(vm_name, "."))
	if err != nil {
		return VM{}
	}
	readVM := VM{}
	err = json.Unmarshal(vmJSON, &readVM)
	if err != nil {
		log.Fatalf("Failed to JSON Unmarshal")
	}
	return readVM

}
func (s *Store) Update(vm *VM) {

}

/* func (s *Store) Delete(vm_name string) {

	vm := s.Get(vm_name)
	if vm != (VM{}) {

	}

}
*/
func (s *Store) FlushAll() {
	s.conn.Do("FLUSHALL")
}

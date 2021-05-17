# OCI Action API

This is a project written in Go to scan Oracle Cloud accounts and execute compute actions (start|stop|restart instances). 

It has three components:
- A redis database to store name of compute instances with their OCID, Region and Account profile.
- A worker node to periodically scan Oracle Cloud account and store the info in a redis database.
- A rest api that will help to find the compute instace by its name previously scanned and execute either start, stop or rstart on demand.

## Requirements
- A OCI account
- GO >= go1.16.2
- Docker >= 20.10.5, build 55c4c88 (Tested in this version, I'm sure will work in older versions ;-) )
- Kubernetes >= v1.19.7 (Tested in this version, I'm sure will work in older versions ;-) )


## Installation Steps

1. Build the images

    ### Build worker node
    ```
    docker build -t davejfranco/oci-action-worker -f worker/Dockerfile .
    ```

    ### Build api node
    ```
    docker build -t davejfranco/oci-action-api .
    ```

2. Login and push into your registry
    ```
    docker login
    docker push davejfranco/oci-action-worker
    docker push davejfranco/oci-action-api
    ```

3. Create K8s secret with your docker login credentials
    ```
    kubectl create secret generic dockerhub --from-file=.dockerconfigjson=/home/dave/.docker/config.json --type=kubernetes.io/dockerconfigjson
    ```
    or if you are using Oracle container registry, you need to create an auth token in your user settings. For more info: https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/managingcredentials.htm 
    
    ```
    kubectl create secret docker-registry ocirsecret --docker-server=iad.ocir.io --docker-username='idlhjo6dp3bd/oracleidentitycloudservice/davefranco1987@gmail.com' --docker-password=$(cat ~/.oci/token) --docker-email='davefranco1987@gmail.com'
    ```

4. Create credential files.
- Create user and group and then apply the following policy to the group
  ```
    - Allow group [group name] to read all-resources in tenancy where request.operation = 'ListCompartments'	
    - Allow group [group name] to manage instance in tenancy where any {request.operation = 'InstanceAction', request.operation = 'ListInstances'}
  ```
  For more details on how to create and manage polocies on OCI: [https://docs.oracle.com/en-us/iaas/Content/Identity/Concepts/policygetstarted.htm](https://)

- Generate config and key pair to authenticate with OCI account, by using the oci-cli is possible to generate both.
  ```
    oci setup config
  ```
  The output should look like this
  ```
  [DEFAULT]
  user=ocid1.user.oc1..bbbbbbbbbbbiarn7vrn62bzfvvjbkvhdmihwsfrenw4zxe7oexm3b2jm6pbaoja
  fingerprint=e8:bf:55:93:c7:63:45:50:57:38:8a:45:d9:c1:a2:db
  key_file=/root/oci_api_key.pem
  tenancy=ocid1.tenancy.oc1..aaaaaaaavl2ndgiiefoo2u4a7atlq2czcwyiu5zzb6rzwwpeyt5o2xmtaxwa
  region=us-ashburn-1
  ```
  Make sure the path to the private key is in /root and name oci_api_key.pem

  For more details on oci-cli follow this link: [https://docs.oracle.com/es-ww/iaas/Content/API/SDKDocs/cliinstall.htm](https://)

5. Create configmaps 

    Edit the file kube/configmaps.yaml with the info of your OCI accounts
    ```
    kubectl create -f kube/configmaps.yaml
    ```
6. Deploy the redis database into your k8s cluster

    ```
    kubectl create -f kube/redis.yaml
    ```

7. Modify container image in api.yaml and worker.yaml with the name of your container image and create resources
    ```
      kubectl create -f /kube/api.ymal
      kubectl create -f /kube/worker.ymal
    ```
## HowTo
Once the resources are created, the microservice will scan comparments, regions and vms; it takes around a couple of mins depending on how many resources and accounts you have.

### Find Compute Instance by Name
  ```
  curl -X GET "http://localhost:8080/oci?name=MyVM"
  ```

### Execute action on a given vm name
  ```
  curl -i "http://localhost:8080/oci" -X POST -d '{"name":"wls-1", "action":"stop"}' -H "Content-Type: application/json"
  ```
  Actions Allowed: start, stop, restart

  Note: both stop and restart will execute softstop and softrestart respectively 
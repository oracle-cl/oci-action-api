# OCI Action API

This is a sample REST API written in Go to execute compute actions in Oracle Cloud Infrastructure

## Requirements
- A OCI account
- GO >= go1.16.2
- Docker >= 20.10.5, build 55c4c88 (Tested in this version, I'm sure will work in older versions ;-) )
- Kubernetes >= v1.19.7 (Tested in this version, I'm sure will work in older versions ;-) )


## Installation Steps

1. Build the image
  ```
  docker build -t davejfranco/oci-action-api .
  ```
2. Login and push into your registry
  ```
  docker login
  docker push davejfranco/oci-action-api
  ```

3. Create K8s secret with your docker login credentials
  ```
  kubectl create secret generic dockerhub --from-file=.dockerconfigjson=/home/dave/.docker/config.json --type=kubernetes.io/dockerconfigjson
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

5. Create configmaps with the config and private key files
  ```
  kubectl create configmap oci-config  --from-file=config
  kubectl create configmap oci-priv-key  --from-file=oci_api_key.pem
  ```

6. Modify container image in kubefile.yaml with the name of your container image and create resources
  ```
  kubectl create -f kubefile
  ```
## HowTo
Once the resources are created, the microservice will scan comparments, regions and vms; it takes around a min depends on how many regions and compartments to be ready.

### Find Compute Instance by Name
  ```
  curl -X GET "http://localhost:8080/oci?name=MyVM"
  ```

### Execute action on a given vm name
  ```
  curl -X POST -H "Content-type: application/json" "http://localhost:8080/oci?name=MyVM&action=start"
  ```
  Actions Allowed: start, stop, restart

  Note: both stop and restart will execute softstop and softrestart respectively 
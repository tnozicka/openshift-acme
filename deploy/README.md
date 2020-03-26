# Deploying the controller

## Issuers
Let's encrypt provides two environments: **live** and **staging**. The environment is chosen by creating the appropriate issuer. 

### Staging
*Staging* is meant for testing the controller or making sure you can try it out without the fear or exhausting your rate limits while trying it out and it will provide you with certificates signed by Let's Encrypt staging CA, making the certs **not trusted**!

### Live
*Live* will provide you with trusted certificates signed by Let's Encrypt CA but has lower rate limits. This is what you want when you're done testing/evaluating the controller.

## Deployment types

### Cluster wide
This deployment will provide certificate management for all namespaces in your cluster. You need elevated (admin) privileges to deploy it.

If you have this repository checked out, deploy it like: 

```bash
oc apply -fdeploy/cluster-wide/{clusterrole,serviceaccount,issuer-letsencrypt-live,deployment}.yaml
oc create clusterrolebinding openshift-acme --clusterrole=openshift-acme --serviceaccount="$( oc project -q ):openshift-acme" --dry-run -o yaml | oc apply -f -
```

If you want to deploy it directly from GitHub use:

```bash
oc apply -fhttps://raw.githubusercontent.com/tnozicka/openshift-acme/master/deploy/cluster-wide/{clusterrole,serviceaccount,issuer-letsencrypt-live,deployment}.yaml
oc create clusterrolebinding openshift-acme --clusterrole=openshift-acme --serviceaccount="$( oc project -q ):openshift-acme" --dry-run -o yaml | oc apply -f -
```


### Single namespace
This deployment will provide certificate management for the namespace it's deployed to. You have to make sure to give the SA correct permissions but you don't have to be cluster-admin. It works fine with regular user privileges.

If you have this repository checked out, deploy it like: 

```bash
oc apply -fdeploy/single-namespace/{role,serviceaccount,issuer-letsencrypt-live,deployment}.yaml
oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="$( oc project -q ):openshift-acme" --dry-run -o yaml | oc apply -f -
```

If you want to deploy it directly from GitHub use:

```bash
oc apply -fhttps://raw.githubusercontent.com/tnozicka/openshift-acme/master/deploy/single-namespace/{role,serviceaccount,issuer-letsencrypt-live,deployment}.yaml
oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="$( oc project -q ):openshift-acme" --dry-run -o yaml | oc apply -f -
```

### Specific namespaces
This deployment will provide certificate management for the namespace it's deployed to and explicitly specified namespaces. You have to make sure to give the SA correct permissions but you don't have to be cluster-admin. It works fine with regular user privileges.

To set up more namespace the deployment needs extra `--namespace=test` flag and you need to give the openshift-acme serviceaccount appropriate privileges for the extra namespace, like in the example bellow for the extra namespace `test`.   

If you have this repository checked out, deploy it like: 

```bash
oc apply -fdeploy/specific-namespaces/{role,serviceaccount,issuer-letsencrypt-live,deployment}.yaml
oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="$( oc project -q ):openshift-acme" --dry-run -o yaml | oc apply -f -
oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="test:openshift-acme" -n "test" --dry-run -o yaml | oc apply -f -
```

If you want to deploy it directly from GitHub use:

```bash
oc apply -fhttps://raw.githubusercontent.com/tnozicka/openshift-acme/master/deploy/specific-namespaces/{role,serviceaccount,issuer-letsencrypt-live,deployment}.yaml
oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="$( oc project -q ):openshift-acme" --dry-run -o yaml | oc apply -f -
oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="test:openshift-acme" -n "test" --dry-run -o yaml | oc apply -f -
```

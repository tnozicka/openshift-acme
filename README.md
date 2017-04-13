[![Build Status](https://travis-ci.org/tnozicka/openshift-acme.svg?branch=master)](https://travis-ci.org/tnozicka/openshift-acme)
# openshift-acme
ACME Controller for OpenShift and Kubernetes cluster.

Controller is provider independent but to start with we would recommend you to use Let's Encrypt (https://letsencrypt.org). (And we have prepared deployments with Let's Encrypt set up as ACME provider for you. You can find it in [deploy folder](/deploy) folder. Also see [section Deploy](#deploy).)

## Status
Beware: this controller is in early development phase. We are working tirelessly to make it more stable and feature rich. But it takes time. We will welcome your help by sending a PR or by testing it and giving us early feedback.

At this moment we only support OpenShift Routes. But the whole controller is designed with Ingresses and other objects in mind so we can add them easily for a future release.

## Enabling ACME certificates for your object
```yaml
metadata:
  annotations:
    kubernetes.io/tls-acme: "true"
```

## Deploy
We have created some deployments to get you started in just a few seconds. (But feel free to create one that suits your needs.)

### OpenShift:
```bash
oc create -fhttps://github.com/tnozicka/openshift-acme/raw/master/deploy/{clusterrole,deploymentconfig-letsencrypt-staging,service}.yaml
```
#### Privileges
Because the controller needs to watch for events across namespaces and write certificate objects to them, depending on the settings, it needs elevated privileges. One way to allow necessary privileges is shown bellow, but you might set up different policy if you like.
```bash
oc adm policy add-cluster-role-to-user acme-controller system:serviceaccount:acme:default
```
If, for some reason, this doesn't work for you, please file an issue and you can fallback to elevated privileges by using the default cluster-admin role:
```bash
oc adm policy add-cluster-role-to-user cluster-admin system:serviceaccount:acme:default
```

### Kubernetes
```bash
kubectl create -fhttps://github.com/tnozicka/openshift-acme/raw/master/deploy/{deployment-letsencrypt-staging,service}.yaml
```

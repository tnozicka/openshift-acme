[![Build Status](https://travis-ci.org/tnozicka/openshift-acme.svg?branch=master)](https://travis-ci.org/tnozicka/openshift-acme)
# openshift-acme
openshift-acme is ACME Controller for OpenShift and Kubernetes clusters. It will automatically provision certficates using ACME protocol and manage their lifecycle (like automatic renewals).

Controller is provider independent but to start with we would recommend you to use Let's Encrypt (https://letsencrypt.org). For more information checkout [section Deploy](#deploy).)

## Enabling ACME certificates for your object
Once openshift-acme controller is running on your cluster all you have to do is annotate your Route or other supported object like this:
```yaml
metadata:
  annotations:
    kubernetes.io/tls-acme: "true"
```

## Deploy
We have created some deployments to get you started in just a few seconds. (But feel free to create one that suits your needs.)

Let's encrypt provides two APIs: **live** and **staging**. There is also an option to deploy the controller to watch the whole cluster or only single namespace depending on what privileges you have.

*Warning*: Whenever you need to switch between those two environments you need to delete the Secret `acme-account` created on your behalf in the same namespace as you've deployed the controller. (Those environments are totally separate making your account invalid when used with the other one.)


### Staging
*staging* is meant for testing the controller or making sure you can try it out without the fear or exhausing your rate limits while trying it out and it will provide you with certificates signed by Let's Encrypt testing CA making the certs **not trusted**!
- [Cluster wide](deploy/letsencrypt-staging/cluster-wide)
- [Single namespace](deploy/letsencrypt-staging/single-namespace)

### Live
*live* will provide you with trusted certificates but has lower rate limits. This is what you want when you're done testing/evaluating the controller

- [Cluster wide](deploy/letsencrypt-live/cluster-wide)
- [Single namespace](deploy/letsencrypt-live/single-namespace)

## Status
openshift-acme just went through a complete rewrite to become a fully fledged controller using all the fancy stuff like shared informers and work queue to make it more reliable.


### Supported objects
#### Routes (OpenShift)
OpenShift Routes are fully supported. 

Also in near future for every route that has certificate provisioned by openshift-acme, the controller will create a Secret containing the certificate to allow you to mount it into pods and enable SSL in the passthrough mode. This will be especially useful for not HTTP based protocols. 


### Roadmap
- Advanced rate limiting
- Creating associated Secrets for Routes containing the certificate
- Ingress (and Kubernetes) support




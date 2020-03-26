[![Donate](https://img.shields.io/badge/Donate-PayPal-green.svg)](https://www.paypal.com/cgi-bin/webscr?cmd=_s-xclick&hosted_button_id=KQE4S78YRTEA6)

# openshift-acme
openshift-acme is ACME Controller for OpenShift and Kubernetes clusters. It will automatically provision certificates using ACME v2 protocol and manage their lifecycle including automatic renewals.

The controller is provider independent but to start with we would recommend you to use Let's Encrypt (https://letsencrypt.org). For more information checkout [section Deploy](#deploy).)

## Enabling ACME certificates for your object
Once openshift-acme controller is running on your cluster all you have to do is annotate your Route or other supported object like this:
```yaml
metadata:
  annotations:
    kubernetes.io/tls-acme: "true"
```

<!--- TODO: Record new one
## Screencast
[![openshift-acme screencast](https://asciinema.org/a/175706.png)](https://asciinema.org/a/175706)
--->

## Deploy
openshift-acme provides multiple options to deploy the controller so you can deploy it even as a regular user in a shared cluster only for specific namespaces you have access to. We intentionally avoid using CRDs which require system:admin privileges.

We have created deployments to get you started in just a few seconds. (But feel free to create one that suits your needs.)
- [Cluster wide](deploy#cluster-wide)
- [Single namespace](deploy#single-namespace)
- [Specific namespaces](deploy#specific-namespaces)

Let's encrypt provides two environments: **live** and **staging**. The environment is chosen based on the issuer ConfigMap that is created.

### Staging
*staging* is meant for testing the controller or making sure you can try it out without the fear or exhausting your rate limits and it will provide you with certificates signed by Let's Encrypt staging CA making the certs **not trusted**!

### Live
*live* will provide you with **trusted certificates** but has lower rate limits. This is what you want when you're done testing/evaluating the controller

## Status
openshift-acme now supports only ACME v2 protocol. For the time of the transition the **old images using ACME v1** are kept in `docker.io/tnozicka/openshift-acme:v0.8.0`. There is no plan to support the old version and while you can still use it until the endpoints are turned off, we advise you to try the new version of the controller and migrate.

### Supported objects
#### Routes (OpenShift)
OpenShift Routes are fully supported.

If you annotate your Route with "acme.openshift.io/secret-name": "<secret_name>", the controller will synchronize the Route certificates into a Secret so you can use SSL in the passthrough mode and mount the secret into pods.

### Roadmap
- Advanced rate limiting (there is now support for basic rate limits)
- Ingress (and Kubernetes) support
- DNS validation support
- CertificateRequests objects (when not using http-01 validation you don't need a Route)
- Operator managing the deployment and upgrades

## Mailing list
https://groups.google.com/d/forum/openshift-acme

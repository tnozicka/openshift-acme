WARNING: **staging** is meant for testing with Let's Encrypt and will provide certificates signed by testing CA making the certs untrusted although with higher rate limits. To get fully valid certificates use **live**.

This deployment will provide certificate management for all namespaces in your cluster. You need elevated (admin) privileges to deploy it.

If you have this repository checked out, deploy it like: 

```bash
oc create -fdeploy/letsencrypt-staging/cluster-wide/{clusterrole,serviceaccount,imagestream,deployment}.yaml
oc adm policy add-cluster-role-to-user openshift-acme -z openshift-acme
```

If you want to deploy it directly from GitHub use:

```bash
oc create -fhttps://raw.githubusercontent.com/tnozicka/openshift-acme/master/deploy/letsencrypt-staging/cluster-wide/{clusterrole,serviceaccount,imagestream,deployment}.yaml
oc adm policy add-cluster-role-to-user openshift-acme -z openshift-acme
```

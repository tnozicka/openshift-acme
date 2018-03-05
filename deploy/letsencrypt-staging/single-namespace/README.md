WARNING: **staging** is meant for testing with Let's Encrypt and will provide certificates signed by testing CA making the certs untrusted although with higher rate limits. To get fully valid certificates use **live**.

This deployment will provide certificate management only for the namespace it's deployed to. It works fine with regular user privileges.

If you have this repository checked out, deploy it like: 

```bash
oc create -fdeploy/letsencrypt-staging/single-namespace/{role,serviceaccount,imagestream,deployment}.yaml
oc policy add-role-to-user openshift-acme --role-namespace="$(oc project --short)" -z openshift-acme
```

If you want to deploy it directly from GitHub use:

```bash
oc create -fhttps://raw.githubusercontent.com/tnozicka/openshift-acme/master/deploy/letsencrypt-staging/single-namespace/{role,serviceaccount,imagestream,deployment}.yaml
oc policy add-role-to-user openshift-acme --role-namespace="$(oc project --short)" -z openshift-acme
```


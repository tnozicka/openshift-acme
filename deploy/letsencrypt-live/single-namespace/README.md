**Live** will provide you with fully valid certificates signed by Let's Encrypt CA.

This deployment will provide certificate management only for the namespace it's deployed to. It works fine with regular user privileges.

If you have this repository checked out, deploy it like: 

```bash
oc create -fdeploy/letsencrypt-live/single-namespace/{role,serviceaccount,imagestream,deployment}.yaml
oc policy add-role-to-user openshift-acme --role-namespace="$(oc project --short)" -z openshift-acme
```

If you want to deploy it directly from GitHub use:

```bash
oc create -fhttps://raw.githubusercontent.com/tnozicka/openshift-acme/master/deploy/letsencrypt-live/single-namespace/{role,serviceaccount,imagestream,deployment}.yaml
oc policy add-role-to-user openshift-acme --role-namespace="$(oc project --short)" -z openshift-acme
```


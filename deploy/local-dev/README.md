# Local development

## Forward your public IP ports to OpenShift router
```bash
ssh -N -R 'localhost:45001:localhost:443' -R 'localhost:45000:localhost:80' your.public.server.io
```

## Forward acme-controller service traffic to your computer
```bash
oc create -f{service,deployment}.yaml
```
TODO: the .ssh/authorized_keys is supposed to be bind-mounted from a Secret.

```bash
ssh -4 -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@`oc get svc acme-controller -n acme -o template --template='{{.spec.clusterIP}}'` -p 2222 -N -R '0.0.0.0:6000:localhost:5000'
```

## Run openshift-acme controller on your computer and develop
```bash
go install -v && go run main.go --kubeconfig=$KUBECONFIG --exposer-ip=`oc get -n acme ep/acme-controller -o template --template '{{ (index (index .subsets 0).addresses 0).ip }}'` --loglevel=5
```


#!/usr/bin/env bash
set -eEuxo pipefail

case $1 in
"cluster-wide")
    ;;
"single-namespace")
    ;;
*)
    echo "bad argument: " + $1
    exit 1
esac


function teardown {
    if [ -n "${ARTIFACT_DIR}" ]; then
        oc logs -n "${PROJECT}" deploy/openshift-acme > "${ARTIFACT_DIR}"/openshift-acme_deploy.log || true
    fi
}
trap teardown ERR EXIT

# The default routes are longer then LE max length of 64 character, we need to create shorter domain
oc new-project test
oc create route edge test --service=none --port=80

router_canonical_hostname=""
until [[ ${router_canonical_hostname} != "" ]]; do
    router_canonical_hostname=$( oc get route test -o go-template --template="{{ or (index .status.ingress 0).routerCanonicalHostname \"\" }}" || true );
done

domain_length=$( echo $(( $( echo ${router_canonical_hostname} | wc -c ) + 1 )) )
echo domain_length: ${domain_length}

prefix_length=$( echo $(( 64 - ${domain_length} )) )
(( ${prefix_length} > 0 ))

prefix=$( cat /dev/urandom | tr -dc 'a-z0-9' | fold -w ${prefix_length} | head -n 1 || [[ $? == 141 ]] )

export TEST_DOMAIN=${prefix}.${router_canonical_hostname}
(( ${prefix_length} <= 64 ))


# Deploy
PROJECT=${PROJECT:-acme-controller}
oc new-project "${PROJECT}"

case $1 in
"cluster-wide")
    oc apply -fdeploy/letsencrypt-staging/cluster-wide/imagestream.yaml
    ;;

"single-namespace")
    oc create user developer
    oc create clusterrolebinding developer --clusterrole=basic-user --user=developer
    oc adm policy add-role-to-user admin developer -n "${PROJECT}"
    # perms sanity checks
    oc --as developer auth can-i create clusterrole && exit 1
    oc --as developer auth can-i create deployment -n "${PROJECT}"

    oc --as developer apply -fdeploy/letsencrypt-staging/single-namespace/imagestream.yaml
    ;;

*)
    exit 1
esac

oc tag -d openshift-acme:latest
oc tag registry.svc.ci.openshift.org/"${OPENSHIFT_BUILD_NAMESPACE}"/pipeline:openshift-acme openshift-acme:latest

case $1 in
"cluster-wide")
    oc create -fdeploy/letsencrypt-staging/cluster-wide/{clusterrole,serviceaccount,deployment}.yaml

    oc adm policy add-cluster-role-to-user openshift-acme -z openshift-acme

    oc adm pod-network make-projects-global "${PROJECT}" || true

    export FIXED_NAMESPACE=""
    ;;

"single-namespace")
    oc --as=developer create -fdeploy/letsencrypt-staging/single-namespace/{role,serviceaccount,deployment}.yaml

    oc --as=developer policy add-role-to-user openshift-acme --role-namespace="${PROJECT}" -z openshift-acme

    export FIXED_NAMESPACE="${PROJECT}"
    ;;

*)
    exit 1
esac


# TODO: use subdomains so the domain differs for every test
export DELETE_ACCOUNT_BETWEEN_STEPS_IN_NAMESPACE=${PROJECT}


tmpFile=$( mktemp )

timeout 10m oc rollout status deploy/openshift-acme

make -j64 test-extended GOFLAGS="-v -race"

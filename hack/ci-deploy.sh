#!/usr/bin/env bash
set -eEuxo pipefail

PROJECT=${PROJECT:-acme-controller}


case $1 in
"cluster-wide")
    ;;
"single-namespace")
    ;&
"specific-namespaces")
    oc create user developer --dry-run -o yaml | oc apply -f -
    oc adm policy add-cluster-role-to-user self-provisioner developer

    token=$( openssl rand -hex 32 )
    oc apply -f - <<EOF
apiVersion: oauth.openshift.io/v1
kind: OAuthAccessToken
metadata:
  name: ${token}
clientName: openshift-challenging-client
userName: developer
userUID: $( oc get user developer --template='{{.metadata.uid}}' )
scopes: ["user:full"]
redirectURI: https://localhost:8443/oauth/token/implicit
EOF
    adminkubeconfig=${KUBECONFIG}
    userkubeconfig=$( mktemp )
    cp "${adminkubeconfig}" "${userkubeconfig}"
    KUBECONFIG=${userkubeconfig}
    oc login --token "${token}"
    ;;
*)
    echo "bad argument: " + $1
    exit 1
esac

oc whoami
oc new-project "${PROJECT}" || oc project "${PROJECT}" 2>/dev/null

deploy_file=$( mktemp )

cat deploy/$1/deployment.yaml | \
    sed -e "s~quay.io/tnozicka/openshift-acme:controller~${CONTROLLER_IMAGE}~" | \
    sed -e "s~quay.io/tnozicka/openshift-acme:exposer~${EXPOSER_IMAGE}~" | \
    sed -e "s~replicas:.*~replicas: 1~" | \
    tee ${deploy_file}

case $1 in
"cluster-wide")
    oc apply -f deploy/$1/clusterrole.yaml
    oc create clusterrolebinding openshift-acme --clusterrole=openshift-acme --serviceaccount="${PROJECT}:openshift-acme" -n "${PROJECT}" --dry-run -o yaml | oc apply -f -
    ;;

"single-namespace")
    oc apply -f deploy/$1/role.yaml -n "${PROJECT}"
    oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="${PROJECT}:openshift-acme" -n "${PROJECT}" --dry-run -o yaml | oc apply -f -
    ;;

"specific-namespaces")
    oc apply -f deploy/$1/role.yaml -n "${PROJECT}"
    oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="${PROJECT}:openshift-acme" -n "${PROJECT}" --dry-run -o yaml | oc apply -f -
    oc new-project "test" || oc project "test" 2>/dev/null
    oc project "${PROJECT}"
    oc apply -f deploy/$1/role.yaml -n "test"
    oc create rolebinding openshift-acme --role=openshift-acme --serviceaccount="${PROJECT}:openshift-acme" -n "test" --dry-run -o yaml | oc apply -f -

    ;;
*)
    exit 1
esac

oc apply -fdeploy/$1/{serviceaccount,issuer-letsencrypt-staging}.yaml -f "${deploy_file}" -n "${PROJECT}"

timeout --foreground 10m oc rollout status deploy/openshift-acme

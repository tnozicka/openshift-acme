#!/usr/bin/env bash
set -eEuxo pipefail

ARTIFACTS_DIR=${PROJECT:-$( mktemp -d )}
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

if [[ "$1" -eq "specific-namespaces" ]]; then
    oc new-project foo
    oc new-project bar
fi

oc whoami
oc new-project "${PROJECT}" || oc project "${PROJECT}" 2>/dev/null

deploy_dir=${ARTIFACTS_DIR}/deploy
mkdir "${deploy_dir}"
cp -r ./deploy/$1/ "${deploy_dir}"

sed -i -e "s~quay.io/tnozicka/openshift-acme:controller~${CONTROLLER_IMAGE}~" \
       -e "s~quay.io/tnozicka/openshift-acme:exposer~${EXPOSER_IMAGE}~" \
       -e "s~replicas:.*~replicas: 1~" \
       "${deploy_dir}"/deployment.yaml

oc apply -f"${deploy_dir}"/*.yaml

timeout --foreground 10m oc rollout status deploy/openshift-acme

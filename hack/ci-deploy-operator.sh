#!/usr/bin/env bash
set -eEuxo pipefail

ARTIFACTS_DIR=${PROJECT:-$( mktemp -d )}
PROJECT=${PROJECT:-acme-operator}

oc whoami
oc new-project "${PROJECT}" || oc project "${PROJECT}" 2>/dev/null


deploy_dir=${ARTIFACTS_DIR}/deploy
mkdir "${deploy_dir}"
cp -RL ./deploy/operator/* "${deploy_dir}"/

sed -i -e "s~quay.io/tnozicka/openshift-acme:controller~${CONTROLLER_IMAGE}~" \
       -e "s~quay.io/tnozicka/openshift-acme:exposer~${EXPOSER_IMAGE}~" \
       -e "s~replicas:.*~replicas: 1~" \
       "${deploy_dir}"/50_deployment.yaml

oc apply -f "${deploy_dir}"/00_crd.yaml
oc wait --for=condition=Established --timeout=1m crd/acmecontrollers.acme.openshift.io
oc apply -f "${deploy_dir}"

timeout --foreground 10m oc rollout status deploy/openshift-acme-operator
oc wait --for=condition=Available --timeout=5m deploy/openshift-acme-operator

oc wait --for=condition=DeploymentAvailable acmecontroller/cluster



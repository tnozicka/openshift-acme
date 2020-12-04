#!/usr/bin/env bash
set -eEuxo pipefail

ARTIFACTS_DIR=${PROJECT:-$( mktemp -d )}
PROJECT=${PROJECT:-acme-operator}

oc whoami
oc new-project "${PROJECT}" || oc project "${PROJECT}" 2>/dev/null


deploy_dir=${ARTIFACTS_DIR}/deploy
mkdir "${deploy_dir}"
cp -RL ./deploy/operator/* "${deploy_dir}"/

sed -i -e "s~quay.io/tnozicka/openshift-acme:operator~${OPERATOR_IMAGE}~" \
       -e "s~quay.io/tnozicka/openshift-acme:controller~${CONTROLLER_IMAGE}~" \
       -e "s~quay.io/tnozicka/openshift-acme:exposer~${EXPOSER_IMAGE}~" \
       -e 's~replicas:.*~replicas: 1~' \
       "${deploy_dir}"/50_deployment.yaml
[[ "$( diff --side-by-side --suppress-common-lines ${deploy_dir}/50_deployment.yaml ./deploy/operator/50_deployment.yaml | wc -l )" -eq "4" ]]

oc apply -f "${deploy_dir}"/00_crd.yaml
oc apply -f "${deploy_dir}"/00_namespace.yaml
oc wait --for=condition=Established --timeout=1m crd/acmecontrollers.acme.openshift.io
oc apply -f"${deploy_dir}"/10_{cr,role,serviceaccount}.yaml
oc apply -f "${deploy_dir}"/20_rolebinding.yaml
oc apply -f"${deploy_dir}"/50_{pdb,deployment}.yaml

timeout --foreground 10m oc rollout status deploy/openshift-acme-operator
oc wait --for=condition=Available --timeout=5m deploy/openshift-acme-operator

oc wait --for=condition=DeploymentAvailable acmecontroller/cluster



#!/bin/bash
set -e
set -u
set -o pipefail

# For comparing dirs we need to make sure files are sorted in consistent order
export LC_COLLATE=C

script_full_path=$(readlink -f $0)
script_dir=$(dirname ${script_full_path})
pushd ${script_dir}/.. 1>/dev/null

outdir=${1:-$(mktemp -d)}

cluster_wide_common_files=$(find ./deploy/cluster-wide -type f -not -path ./deploy/cluster-wide/clusterrole.yaml -printf '%P ')
single_namespace_common_files=$(find ./deploy/single-namespace -type f -not -path ./deploy/single-namespace/role.yaml -printf '%P ')
specific_namespaces_common_files=$(find ./deploy/specific-namespaces -type f -not -path ./deploy/specific-namespaces/role.yaml -printf '%P ')

diff <(echo "${cluster_wide_common_files}") <(echo "${single_namespace_common_files}")
for file in ${cluster_wide_common_files}; do
    diff deploy/cluster-wide/${file} deploy/single-namespace/${file} > ${outdir}/single_namespace-${file}.diff || [[ "$?" == 1 ]]
done
diff deploy/cluster-wide/clusterrole.yaml deploy/single-namespace/role.yaml > ${outdir}/single_namespace-roles.diff || [[ $? == 1 ]]

diff <(echo "${cluster_wide_common_files}") <(echo "${specific_namespaces_common_files}")
for file in ${cluster_wide_common_files}; do
    diff deploy/cluster-wide/${file} deploy/specific-namespaces/${file} > ${outdir}/specific_namespaces-${file}.diff || [[ "$?" == 1 ]]
done
diff deploy/cluster-wide/clusterrole.yaml deploy/specific-namespaces/role.yaml > ${outdir}/specific_namespaces-roles.diff || [[ $? == 1 ]]


diff -r deploy/.diffs ${outdir}
